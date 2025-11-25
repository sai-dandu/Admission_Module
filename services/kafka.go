package services

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"sync"
	"time"

	"admission-module/config"
	"admission-module/logger"

	"github.com/segmentio/kafka-go"
)

var (
	producer        *kafka.Writer
	producerMutex   sync.Mutex
	isConnected     bool
	consumer        *kafka.Reader
	consumerMutex   sync.Mutex
	consumerRunning bool
	stopConsumer    chan bool
)

// InitProducer initializes a Kafka writer using brokers from the config
func InitProducer() {
	producerMutex.Lock()
	defer producerMutex.Unlock()

	if config.AppConfig.KafkaBrokers == "" {
		logger.Info("Kafka is disabled (KAFKA_BROKERS is empty)")
		return
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
		logger.Warn("No valid Kafka brokers configured")
		return
	}

	// Attempt to create required topics
	ensureTopicsExist(validBrokers)

	producer = &kafka.Writer{
		Addr:     kafka.TCP(validBrokers...),
		Balancer: &kafka.LeastBytes{},
		Async:    false,
		// Set a reasonable write timeout
		WriteTimeout: 10 * time.Second,
		// Allow up to 10 retries
		RequiredAcks: kafka.RequireAll,
	}

	logger.Info("✓ Kafka producer initialized. Brokers=%v, Topic=%s", validBrokers, config.AppConfig.KafkaTopic)
	isConnected = true
}

// ensureTopicsExist creates Kafka topics if they don't already exist
// This runs in a background goroutine to avoid blocking initialization
func ensureTopicsExist(brokers []string) {
	go func() {
		// Retry logic for topic creation with exponential backoff
		maxRetries := 5
		for attempt := 0; attempt < maxRetries; attempt++ {
			// Give brokers time to stabilize
			waitTime := time.Duration(math.Pow(2, float64(attempt))) * time.Second
			if attempt > 0 {
				time.Sleep(waitTime)
			} else {
				time.Sleep(1 * time.Second) // Initial wait
			}

			conn, err := kafka.Dial("tcp", brokers[0])
			if err != nil {
				if attempt == maxRetries-1 {
					logger.Warn("Could not connect to Kafka broker for topic creation after %d attempts: %v (Kafka topics may need manual creation)", maxRetries, err)
				}
				continue
			}

			requiredTopics := []string{"payments", "applications", "emails", "interviews"}
			successCount := 0

			for _, topic := range requiredTopics {
				// Try to create it - if it exists, CreateTopics will return an error which we ignore
				err := conn.CreateTopics(kafka.TopicConfig{
					Topic:             topic,
					NumPartitions:     1,
					ReplicationFactor: 1,
				})

				if err != nil {
					// Check if it's "topic already exists" error - that's OK
					if strings.Contains(err.Error(), "already exists") {
						logger.Info("✓ Kafka topic '%s' already exists", topic)
						successCount++
					}
				} else {
					logger.Info("✓ Created Kafka topic '%s'", topic)
					successCount++
				}
			}

			conn.Close()

			// If we created or found all topics, we're done
			if successCount >= len(requiredTopics) {
				logger.Info("✓ All required Kafka topics are available")
				return
			}

			// Otherwise retry
			if attempt < maxRetries-1 {
				continue
			}
		}

	}()
}

// Publish marshals value to JSON and publishes to the given topic with key
// Uses exponential backoff retry logic (3 attempts)
func Publish(topic, key string, value interface{}) error {
	producerMutex.Lock()
	if producer == nil && config.AppConfig.KafkaBrokers != "" {
		producerMutex.Unlock()
		InitProducer()
		producerMutex.Lock()
	}
	defer producerMutex.Unlock()

	// If Kafka is disabled or not initialized, skip publishing (best-effort)
	if producer == nil || config.AppConfig.KafkaBrokers == "" {
		logger.Warn("Kafka producer not initialized, skipping publish to topic: %s", topic)
		return nil
	}

	payload, err := json.Marshal(value)
	if err != nil {
		logger.Error("Error marshaling Kafka message: %v", err)
		return err
	}

	logger.Info("Publishing to Kafka topic: %s with key: %s, payload size: %d bytes", topic, key, len(payload))

	msg := kafka.Message{
		Topic: topic,
		Key:   []byte(key),
		Value: payload,
	}

	// Retry with exponential backoff
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := producer.WriteMessages(ctx, msg)
		cancel()

		if err == nil {
			logger.Info("✅ Successfully published to Kafka topic: %s", topic)
			isConnected = true
			return nil
		}

		lastErr = err
		if attempt < 2 {
			backoffTime := time.Duration(math.Pow(2, float64(attempt))) * time.Second
			logger.Warn("Kafka publish attempt %d/%d failed, retrying in %v: %v", attempt+1, 3, backoffTime, err)
			time.Sleep(backoffTime)
		} else {
			logger.Error("Kafka publish failed after 3 attempts: %v", err)
		}
		isConnected = false
	}

	return lastErr
}

// IsConnected returns true if Kafka producer is connected and ready
func IsConnected() bool {
	producerMutex.Lock()
	defer producerMutex.Unlock()
	return isConnected && producer != nil
}

// Close gracefully closes the Kafka producer and consumer
func Close() error {
	producerMutex.Lock()
	defer producerMutex.Unlock()

	if producer != nil {
		return producer.Close()
	}
	return nil
}

// ============================================================================
// KAFKA CONSUMER IMPLEMENTATION
// ============================================================================

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
	// Consumer Group: "admission-module-consumer-group" - identifies this service as a consumer
	consumer = kafka.NewReader(kafka.ReaderConfig{
		Brokers:          validBrokers,
		Topic:            topics[0], // kafka-go reads from one topic at a time, read first topic
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

// It continuously reads messages from Kafka and processes them
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
func handleKafkaMessage(msg kafka.Message) {
	logger.Info("📨 Received message from topic: %s, partition: %d, offset: %d", msg.Topic, msg.Partition, msg.Offset)
	logger.Info("   Key: %s, Size: %d bytes", string(msg.Key), len(msg.Value))

	// Parse the message value as JSON
	var eventData map[string]interface{}
	err := json.Unmarshal(msg.Value, &eventData)
	if err != nil {
		logger.Error("Error unmarshaling message: %v", err)
		return
	}

	logger.Info("   Event type: %v", eventData["event"])

	// Route to appropriate handler based on event type
	eventType, ok := eventData["event"].(string)
	if !ok {
		logger.Warn("Message does not contain event type")
		return
	}

	// Handle different event types
	switch eventType {
	case "lead.created":
		handleLeadCreated(eventData)
	case "payment.initiated":
		handlePaymentInitiated(eventData)
	case "payment.verified":
		handlePaymentVerified(eventData)
	case "email.sent":
		handleEmailSent(eventData)
	case "application.submitted":
		handleApplicationSubmitted(eventData)
	default:
		logger.Warn("Unknown event type: %s", eventType)
	}
}

// ============================================================================
// EVENT HANDLERS - Process different event types
// ============================================================================

// handleLeadCreated processes lead.created events
func handleLeadCreated(event map[string]interface{}) {
	logger.Info("🆕 Processing lead.created event:")
	logger.Info("   Student ID: %v", event["student_id"])
	logger.Info("   Name: %v", event["name"])
	logger.Info("   Email: %v", event["email"])
	logger.Info("   Counselor: %v", event["counselor_name"])

}

// handlePaymentInitiated processes payment.initiated events
func handlePaymentInitiated(event map[string]interface{}) {
	logger.Info("💳 Processing payment.initiated event:")
	logger.Info("   Student ID: %v", event["student_id"])
	logger.Info("   Order ID: %v", event["order_id"])
	logger.Info("   Amount: %v %v", event["amount"], event["currency"])
	logger.Info("   Payment Type: %v", event["payment_type"])

}

// handlePaymentVerified processes payment.verified events
func handlePaymentVerified(event map[string]interface{}) {
	logger.Info("✅ Processing payment.verified event:")
	logger.Info("   Student ID: %v", event["student_id"])
	logger.Info("   Payment ID: %v", event["payment_id"])
	logger.Info("   Status: %v", event["status"])

}

// handleEmailSent processes email.sent events
func handleEmailSent(event map[string]interface{}) {
	logger.Info("📧 Processing email.sent event:")
	logger.Info("   Recipient: %v", event["recipient"])
	logger.Info("   Subject: %v", event["subject"])

}

// handleApplicationSubmitted processes application events
func handleApplicationSubmitted(event map[string]interface{}) {
	logger.Info("📝 Processing application event:")
	logger.Info("   Student ID: %v", event["student_id"])
	logger.Info("   Course: %v", event["course"])
	logger.Info("   Status: %v", event["status"])

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
