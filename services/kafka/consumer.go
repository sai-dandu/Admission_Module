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

	// Create a new consumer reader
	// Note: kafka-go's Reader reads from a single topic at a time
	// For simplicity, we subscribe to the first topic; in production, use separate consumers per topic or a topic pattern
	consumer = kafka.NewReader(kafka.ReaderConfig{
		Brokers:          validBrokers,
		Topic:            topics[0], // kafka-go reads from one topic at a time
		GroupID:          "admission-module-consumer-group",
		StartOffset:      -1,          // -1 = OffsetNewest (start from latest messages)
		CommitInterval:   time.Second, // Commit offsets every second
		MaxBytes:         10e6,        // 10MB max per fetch
		SessionTimeout:   20 * time.Second,
		ReadBackoffMin:   100 * time.Millisecond,
		ReadBackoffMax:   1 * time.Second,
		QueueCapacity:    100,
		RebalanceTimeout: 60 * time.Second,
	})

	stopConsumer = make(chan bool)
	logger.Info("Kafka consumer initialized. Brokers=%v, Topics=%v, ConsumerGroup=admission-module-consumer-group", validBrokers, topics)
	return nil
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
		// Send to DLQ for invalid event type
		_ = SendToDLQ(msg.Topic, string(msg.Key), msg.Value, "Message does not contain valid event type")
		return false
	}

	// Handle different event types with error handling
	var handlerErr error
	switch eventType {
	case "lead.created":
		handlerErr = handleLeadCreated(eventData)
	case "payment.initiated":
		handlerErr = handlePaymentInitiated(eventData)
	case "payment.verified":
		handlerErr = handlePaymentVerified(eventData)
	case "email.sent", "email.acceptance":
		handlerErr = handleEmailSent(eventData)
	case "application.submitted":
		handlerErr = handleApplicationSubmitted(eventData)
	case "interview.scheduled", "interview.schedule":
		handlerErr = handleInterviewScheduled(eventData)
	default:
		logger.Warn("Unknown event type: %s", eventType)
		_ = SendToDLQ(msg.Topic, string(msg.Key), msg.Value, "Unknown event type: "+eventType)
		return false
	}

	// If handler returned an error, send to DLQ
	if handlerErr != nil {
		logger.Error("Error handling event type %s: %v", eventType, handlerErr)
		_ = SendToDLQ(msg.Topic, string(msg.Key), msg.Value, "Handler error: "+handlerErr.Error())
		return false
	}

	// Success! Message was processed without errors
	return true
}

// ============================================================================
// EVENT HANDLERS - Process different event types
// ============================================================================

// handleLeadCreated processes lead.created events
func handleLeadCreated(event map[string]interface{}) error {
	return nil
}

// handlePaymentInitiated processes payment.initiated events
func handlePaymentInitiated(event map[string]interface{}) error {
	return nil
}

// handlePaymentVerified processes payment.verified events
func handlePaymentVerified(event map[string]interface{}) error {
	return nil
}

// handleEmailSent processes email.sent events
func handleEmailSent(event map[string]interface{}) error {
	return nil
}

// handleApplicationSubmitted processes application events
func handleApplicationSubmitted(event map[string]interface{}) error {
	return nil
}

// handleInterviewScheduled processes interview.scheduled events
func handleInterviewScheduled(event map[string]interface{}) error {
	email, ok := event["email"].(string)
	if !ok || email == "" {
		return fmt.Errorf("invalid email")
	}

	meetLink, ok := event["meet_link"].(string)
	if !ok || meetLink == "" {
		return fmt.Errorf("invalid meet_link")
	}

	// Note: SendEmail is called from parent services package
	// This will be resolved through the services initialization
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
