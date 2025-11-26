package kafka

import (
	"admission-module/config"
	"admission-module/logger"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
)

var (
	consumer        *kafka.Reader
	consumerMutex   sync.Mutex
	consumerRunning bool
	stopConsumer    chan bool
	// emailProcessor is a callback to handle email sending from Kafka consumer
	emailProcessor func(map[string]interface{}) error
	// interviewScheduler is a callback to handle interview scheduling from Kafka consumer
	interviewScheduler func(int, string) error
)

// InitConsumer initializes a Kafka reader (consumer) for specified topics
// This creates a consumer group that reads messages from Kafka topics
func InitConsumer(topics []string) error {
	consumerMutex.Lock()
	defer consumerMutex.Unlock()

	if config.AppConfig.KafkaBrokers == "" {
		logger.Info("Kafka consumer is disabled (KAFKA_BROKERS is empty)")
		return nil
	}

	brokers := strings.Split(config.AppConfig.KafkaBrokers, ",")

	// Validate brokers
	var validBrokers []string
	for _, b := range brokers {
		if b := strings.TrimSpace(b); b != "" {
			validBrokers = append(validBrokers, b)
		}
	}

	if len(validBrokers) == 0 {
		logger.Warn("No valid Kafka brokers configured for consumer")
		return nil
	}

	// Listen specifically to "emails" topic for email events
	emailTopic := "emails"
	consumer = kafka.NewReader(kafka.ReaderConfig{
		Brokers:          validBrokers,
		Topic:            emailTopic,
		GroupID:          "admission-module-consumer-group",
		StartOffset:      -1,
		CommitInterval:   time.Second,
		MaxBytes:         10e6,
		SessionTimeout:   20 * time.Second,
		ReadBackoffMin:   100 * time.Millisecond,
		ReadBackoffMax:   1 * time.Second,
		QueueCapacity:    100,
		RebalanceTimeout: 60 * time.Second,
	})

	stopConsumer = make(chan bool)
	logger.Info("Kafka consumer initialized. Brokers=%v, Topic=%s, ConsumerGroup=admission-module-consumer-group", validBrokers, emailTopic)
	return nil
}

// RegisterEmailProcessor registers the callback function that handles email.send events
func RegisterEmailProcessor(fn func(map[string]interface{}) error) {
	consumerMutex.Lock()
	defer consumerMutex.Unlock()
	emailProcessor = fn
	logger.Info("Email processor registered")
}

// RegisterInterviewScheduler registers the callback function that handles interview.schedule events
func RegisterInterviewScheduler(fn func(int, string) error) {
	consumerMutex.Lock()
	defer consumerMutex.Unlock()
	interviewScheduler = fn
	logger.Info("Interview scheduler registered")
}

// StartConsumer starts consuming messages in a separate goroutine
// This runs continuously until StopConsumer() is called
func StartConsumer() {
	consumerMutex.Lock()
	if consumer == nil {
		consumerMutex.Unlock()
		logger.Warn("Consumer not initialized, cannot start")
		return
	}
	if consumerRunning {
		consumerMutex.Unlock()
		logger.Warn("Consumer already running")
		return
	}
	consumerRunning = true
	consumerMutex.Unlock()

	// Run consumer in a goroutine so it doesn't block the main server
	go consumeMessages()
	logger.Info("✅ Kafka consumer started")
}

// consumeMessages continuously reads messages from Kafka and processes them
func consumeMessages() {
	defer func() {
		consumerMutex.Lock()
		consumerRunning = false
		consumerMutex.Unlock()
	}()

	// Allow time for broker to stabilize
	time.Sleep(2 * time.Second)

	for {
		select {
		case <-stopConsumer:
			logger.Info("Consumer stop signal received")
			return
		default:
			// Read the next message with timeout
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			msg, err := consumer.ReadMessage(ctx)
			cancel()

			if err != nil {
				// Silently ignore expected errors (no messages or timeout)
				if err == context.DeadlineExceeded || err.Error() == "EOF" {
					continue
				}
				// Silently ignore group coordinator startup errors
				if strings.Contains(err.Error(), "Group Coordinator Not Available") {
					time.Sleep(500 * time.Millisecond)
					continue
				}
				// For other errors, silently retry with backoff
				time.Sleep(1 * time.Second)
				continue
			}

			// Process the message
			handleKafkaMessage(msg)
		}
	}
}

// handleKafkaMessage processes incoming Kafka messages
// This is where you handle different event types
// On error, messages are sent to the DLQ
func handleKafkaMessage(msg kafka.Message) {
	_ = HandleKafkaMessageForRetry(msg)
}

// HandleKafkaMessageForRetry processes incoming Kafka messages and returns whether it was successful
// Returns true if message was processed successfully (not sent to DLQ)
// Returns false if message was sent to DLQ
func HandleKafkaMessageForRetry(msg kafka.Message) bool {
	// Parse the message value as JSON
	var eventData map[string]interface{}
	err := json.Unmarshal(msg.Value, &eventData)
	if err != nil {
		logger.Error("Error unmarshaling message: %v", err)
		// Send to DLQ on unmarshal error
		_ = SendToDLQ(msg.Topic, string(msg.Key), msg.Value, "Failed to unmarshal JSON: "+err.Error())
		return false
	}

	logger.Info("Event type: %v", eventData["event"])

	// Route to appropriate handler based on event type
	eventType, ok := eventData["event"].(string)
	if !ok {
		logger.Warn("Message does not contain event type")
		_ = SendToDLQ(msg.Topic, string(msg.Key), msg.Value, "Message does not contain valid event type")
		return false
	}

	var handlerErr error
	switch eventType {
	case "email.send":
		handlerErr = handleEmailSend(eventData)
	case "interview.schedule":
		handlerErr = handleInterviewSchedule(eventData)
	case "email.sent", "email.acceptance":
		handlerErr = handleEmailSentTracking(eventData)
	default:
		logger.Warn("Unknown event type: %s", eventType)
		_ = SendToDLQ(msg.Topic, string(msg.Key), msg.Value, "Unknown event type: "+eventType)
		return false
	}

	if handlerErr != nil {
		logger.Error("Error handling event type %s: %v", eventType, handlerErr)
		_ = SendToDLQ(msg.Topic, string(msg.Key), msg.Value, "Handler error: "+handlerErr.Error())
		return false
	}

	return true
}

// handleEmailSend processes email.send events
func handleEmailSend(event map[string]interface{}) error {
	recipient, ok := event["recipient"].(string)
	if !ok || recipient == "" {
		return fmt.Errorf("invalid recipient in email event")
	}

	subject, ok := event["subject"].(string)
	if !ok || subject == "" {
		return fmt.Errorf("invalid subject in email event")
	}

	body, ok := event["body"].(string)
	if !ok || body == "" {
		return fmt.Errorf("invalid body in email event")
	}

	var attachment []string
	if att, ok := event["attachment"].(string); ok && att != "" {
		attachment = append(attachment, att)
	}

	logger.Info("📧 Sending email - Recipient: %s, Subject: %s", recipient, subject)

	consumerMutex.Lock()
	processor := emailProcessor
	consumerMutex.Unlock()

	if processor != nil {
		return processor(event)
	}

	return fmt.Errorf("email processor not registered")
}

// handleInterviewSchedule processes interview.schedule events from payment webhook
// This handler sends interview scheduling emails via the interview scheduler callback
func handleInterviewSchedule(event map[string]interface{}) error {
	studentID, ok := event["student_id"].(float64)
	if !ok {
		return fmt.Errorf("invalid student_id in interview schedule event")
	}

	studentName, ok := event["name"].(string)
	if !ok || studentName == "" {
		return fmt.Errorf("invalid student name in interview schedule event")
	}

	studentEmail, ok := event["email"].(string)
	if !ok || studentEmail == "" {
		return fmt.Errorf("invalid student email in interview schedule event")
	}

	logger.Info("Interview scheduling for student: %d, Email: %s", int(studentID), studentEmail)

	// Call the registered interview scheduler callback to handle meeting link generation and email sending
	consumerMutex.Lock()
	scheduler := interviewScheduler
	consumerMutex.Unlock()

	if scheduler != nil {
		return scheduler(int(studentID), studentEmail)
	}

	return fmt.Errorf("interview scheduler not registered")
}

// handleEmailSentTracking processes email tracking events
func handleEmailSentTracking(event map[string]interface{}) error {
	logger.Info("📧 Email tracking - Event: %v", event["event"])
	return nil
}

// StopConsumer stops the consumer gracefully
func StopConsumer() error {
	consumerMutex.Lock()
	defer consumerMutex.Unlock()

	if !consumerRunning || consumer == nil {
		logger.Warn("Consumer not running")
		return nil
	}

	// Signal the consumer to stop
	close(stopConsumer)

	// Close the consumer reader
	if err := consumer.Close(); err != nil {
		logger.Error("Error closing consumer: %v", err)
		return err
	}

	logger.Info("✅ Kafka consumer stopped")
	return nil
}

// IsConsumerRunning returns true if the consumer is actively running
func IsConsumerRunning() bool {
	consumerMutex.Lock()
	defer consumerMutex.Unlock()
	return consumerRunning && consumer != nil
}
