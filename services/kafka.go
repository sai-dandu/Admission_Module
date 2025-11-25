package services

import (
	"admission-module/config"
	"admission-module/db"
	"admission-module/logger"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

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
	dlqProducer     *kafka.Writer
	dlqMutex        sync.Mutex
	dlqRetryTicker  *time.Ticker
	stopDLQRetry    chan bool
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

	// Send to DLQ if all retries failed
	logger.Info("📤 Sending failed message to DLQ. Topic: %s, Key: %s", topic, key)
	if dlqErr := SendToDLQ(topic, key, payload, lastErr.Error()); dlqErr != nil {
		logger.Error("Failed to send message to DLQ: %v", dlqErr)
	}

	return lastErr
}

// IsConnected returns true if Kafka producer is connected and ready
func IsConnected() bool {
	producerMutex.Lock()
	defer producerMutex.Unlock()
	return isConnected && producer != nil
}

// ============================================================================
// DLQ PRODUCER - Send failed messages to Dead Letter Queue
// ============================================================================

// InitDLQProducer initializes a Kafka writer for the DLQ topic
func InitDLQProducer() {
	dlqMutex.Lock()
	defer dlqMutex.Unlock()

	if config.AppConfig.KafkaBrokers == "" {
		logger.Info("Kafka DLQ is disabled (KAFKA_BROKERS is empty)")
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
		logger.Warn("No valid Kafka brokers configured for DLQ")
		return
	}

	dlqProducer = &kafka.Writer{
		Addr:         kafka.TCP(validBrokers...),
		Topic:        config.AppConfig.KafkaDLQTopic,
		Balancer:     &kafka.LeastBytes{},
		Async:        false,
		WriteTimeout: 10 * time.Second,
		RequiredAcks: kafka.RequireAll,
	}

	logger.Info("Kafka DLQ producer initialized. Brokers=%v, DLQ Topic=%s", validBrokers, config.AppConfig.KafkaDLQTopic)
}

// SendToDLQ publishes a failed message to the Dead Letter Queue
// Stores both in Kafka and in database for later retrieval
func SendToDLQ(topic, key string, value []byte, errorMsg string) error {
	dlqMutex.Lock()
	if dlqProducer == nil && config.AppConfig.KafkaBrokers != "" {
		dlqMutex.Unlock()
		InitDLQProducer()
		dlqMutex.Lock()
	}
	defer dlqMutex.Unlock()

	if dlqProducer == nil || config.AppConfig.KafkaBrokers == "" {
		logger.Warn("DLQ producer not initialized, storing failed message in database only")
	} else {
		// Create DLQ message envelope
		dlqMessage := map[string]interface{}{
			"original_topic": topic,
			"original_key":   key,
			"original_value": string(value),
			"error_message":  errorMsg,
			"timestamp":      time.Now().Unix(),
			"failure_reason": "Processing failed",
		}

		dlqPayload, err := json.Marshal(dlqMessage)
		if err != nil {
			logger.Error("Error marshaling DLQ message: %v", err)
			return err
		}

		msg := kafka.Message{
			Key:   []byte(key),
			Value: dlqPayload,
		}

		// Send to DLQ with retries
		for attempt := 0; attempt < 3; attempt++ {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := dlqProducer.WriteMessages(ctx, msg)
			cancel()

			if err == nil {
				logger.Info("✅ Message sent to DLQ topic: %s", config.AppConfig.KafkaDLQTopic)
				break
			}

			if attempt < 2 {
				logger.Warn("DLQ publish attempt %d/3 failed, retrying: %v", attempt+1, err)
				time.Sleep(time.Duration(attempt+1) * time.Second)
			} else {
				logger.Error("Failed to send to DLQ after 3 attempts: %v", err)
			}
		}
	}

	// Always store in database for persistence
	return StoreDLQMessage(topic, key, value, errorMsg)
}

// StoreDLQMessage stores a failed message in the database
func StoreDLQMessage(topic, key string, value []byte, errorMsg string) error {
	// Get database connection from your db package
	// This is a placeholder - adjust based on your actual db setup
	db := getDBConnection()
	if db == nil {
		logger.Warn("Database connection not available for DLQ storage")
		return nil
	}

	query := `
		INSERT INTO dlq_messages (message_id, topic, key, value, error_message, created_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, NOW())
		ON CONFLICT (message_id) DO NOTHING
	`

	_, err := db.Exec(query, topic, key, string(value), errorMsg)
	if err != nil {
		logger.Error("Error storing DLQ message in database: %v", err)
		return err
	}

	logger.Info("✅ DLQ message stored in database. Topic: %s, Key: %s", topic, key)
	return nil
}

// GetDLQMessages retrieves unresolved DLQ messages from database
func GetDLQMessages(limit int) ([]map[string]interface{}, error) {
	db := getDBConnection()
	if db == nil {
		return nil, nil
	}

	query := `
		SELECT id, message_id, topic, key, value, error_message, retry_count, created_at
		FROM dlq_messages
		WHERE resolved = FALSE
		ORDER BY created_at DESC
		LIMIT $1
	`

	rows, err := db.Query(query, limit)
	if err != nil {
		logger.Error("Error querying DLQ messages: %v", err)
		return nil, err
	}
	defer rows.Close()

	var messages []map[string]interface{}
	for rows.Next() {
		var id int
		var messageID, topic, key string
		var value, errorMsg string
		var retryCount int
		var createdAt time.Time

		if err := rows.Scan(&id, &messageID, &topic, &key, &value, &errorMsg, &retryCount, &createdAt); err != nil {
			logger.Error("Error scanning DLQ message: %v", err)
			continue
		}

		messages = append(messages, map[string]interface{}{
			"id":            id,
			"message_id":    messageID,
			"topic":         topic,
			"key":           key,
			"value":         value,
			"error_message": errorMsg,
			"retry_count":   retryCount,
			"created_at":    createdAt,
		})
	}

	return messages, nil
}

// RetryDLQMessage attempts to reprocess a DLQ message
func RetryDLQMessage(messageID string) error {
	db := getDBConnection()
	if db == nil {
		return nil
	}

	query := `
		SELECT value, topic, key FROM dlq_messages WHERE message_id = $1
	`

	var value, topic, key string
	err := db.QueryRow(query, messageID).Scan(&value, &topic, &key)
	if err != nil {
		logger.Error("Error retrieving DLQ message for retry: %v", err)
		return err
	}

	// Attempt to reprocess
	var eventData map[string]interface{}
	if err := json.Unmarshal([]byte(value), &eventData); err != nil {
		logger.Error("Error unmarshaling DLQ message for retry: %v", err)
		return err
	}

	// Reprocess the message and check if successful
	wasSuccessful := handleKafkaMessageForRetry(kafka.Message{
		Topic: topic,
		Key:   []byte(key),
		Value: []byte(value),
	})

	// Update retry count and resolve only if successful
	var updateQuery string
	if wasSuccessful {
		updateQuery = `
			UPDATE dlq_messages
			SET retry_count = retry_count + 1, last_retry_at = NOW(), resolved = TRUE, resolved_at = NOW(), notes = 'Manually retried successfully'
			WHERE message_id = $1
		`
		logger.Info("✅ DLQ message %s marked as resolved (manual retry successful)", messageID)
	} else {
		updateQuery = `
			UPDATE dlq_messages
			SET retry_count = retry_count + 1, last_retry_at = NOW()
			WHERE message_id = $1
		`
		logger.Info("ℹ️ DLQ message %s retry count incremented (retry failed, sent back to DLQ)", messageID)
	}
	_, err = db.Exec(updateQuery, messageID)
	return err
}

// ResolveDLQMessage marks a DLQ message as resolved
func ResolveDLQMessage(messageID string, notes string) error {
	db := getDBConnection()
	if db == nil {
		return nil
	}

	query := `
		UPDATE dlq_messages
		SET resolved = TRUE, resolved_at = NOW(), notes = $2
		WHERE message_id = $1
	`

	_, err := db.Exec(query, messageID, notes)
	if err != nil {
		logger.Error("Error resolving DLQ message: %v", err)
		return err
	}

	logger.Info("✅ DLQ message %s marked as resolved", messageID)
	return nil
}

// GetDLQStats retrieves statistics about DLQ messages
func GetDLQStats() (map[string]interface{}, error) {
	db := getDBConnection()
	if db == nil {
		return nil, nil
	}

	var totalMessages, unresolvedMessages, resolvedMessages int

	// Get total DLQ messages
	err := db.QueryRow("SELECT COUNT(*) FROM dlq_messages").Scan(&totalMessages)
	if err != nil {
		logger.Error("Error getting total DLQ messages count: %v", err)
		return nil, err
	}

	// Get unresolved DLQ messages
	err = db.QueryRow("SELECT COUNT(*) FROM dlq_messages WHERE resolved = FALSE").Scan(&unresolvedMessages)
	if err != nil {
		logger.Error("Error getting unresolved DLQ messages count: %v", err)
		return nil, err
	}

	// Get resolved DLQ messages
	err = db.QueryRow("SELECT COUNT(*) FROM dlq_messages WHERE resolved = TRUE").Scan(&resolvedMessages)
	if err != nil {
		logger.Error("Error getting resolved DLQ messages count: %v", err)
		return nil, err
	}

	return map[string]interface{}{
		"total_dlq_messages":  totalMessages,
		"unresolved_messages": unresolvedMessages,
		"resolved_messages":   resolvedMessages,
	}, nil
}

// StartDLQAutoRetry starts a background goroutine that automatically retries failed DLQ messages
// Retries unresolved messages every 10 seconds for testing (change to 5*time.Minute in production)
func StartDLQAutoRetry() {
	dlqRetryTicker = time.NewTicker(10 * time.Second)
	stopDLQRetry = make(chan bool)

	go func() {
		logger.Info("🔄 DLQ auto-retry goroutine started")
		for {
			select {
			case <-dlqRetryTicker.C:
				retryUnresolvedDLQMessages()
			case <-stopDLQRetry:
				logger.Info("DLQ auto-retry stopped")
				return
			}
		}
	}()

	logger.Info("✅ DLQ auto-retry scheduler initialized (checks every 10 seconds)")
}

// retryUnresolvedDLQMessages retrieves unresolved messages and attempts to retry them
func retryUnresolvedDLQMessages() {
	db := getDBConnection()
	if db == nil {
		logger.Warn("Database connection not available for DLQ auto-retry")
		return
	}

	logger.Info("🔄 DLQ auto-retry cycle started...")

	query := `
		SELECT message_id, value, topic, key, retry_count, max_retries
		FROM dlq_messages
		WHERE resolved = FALSE AND retry_count < max_retries
		ORDER BY created_at ASC
		LIMIT 10
	`

	rows, err := db.Query(query)
	if err != nil {
		logger.Error("Error querying unresolved DLQ messages for retry: %v", err)
		return
	}
	defer rows.Close()

	retryCount := 0
	successCount := 0
	for rows.Next() {
		var messageID, value, topic, key string
		var retryAttempts, maxRetries int

		if err := rows.Scan(&messageID, &value, &topic, &key, &retryAttempts, &maxRetries); err != nil {
			logger.Error("Error scanning DLQ message for retry: %v", err)
			continue
		}

		logger.Info("🔄 Auto-retrying DLQ message %s (attempt %d/%d)", messageID, retryAttempts+1, maxRetries)

		// Attempt to reprocess the message - capture success/failure
		// We'll treat successful reprocessing (no SendToDLQ call) as success
		wasReprocessed := handleKafkaMessageForRetry(kafka.Message{
			Topic: topic,
			Key:   []byte(key),
			Value: []byte(value),
		})

		logger.Info("🔍 Reprocessing result for message %s: success=%v", messageID, wasReprocessed)

		// Update retry count and mark as resolved if successful
		if wasReprocessed {
			updateQuery := `
				UPDATE dlq_messages
				SET retry_count = retry_count + 1, last_retry_at = NOW(), resolved = TRUE, resolved_at = NOW(), notes = 'Auto-retried successfully'
				WHERE message_id = $1
			`
			if _, err := db.Exec(updateQuery, messageID); err != nil {
				logger.Error("Error updating DLQ message as resolved %s: %v", messageID, err)
			} else {
				logger.Info("✅ DLQ message %s marked as resolved (auto-retry)", messageID)
				successCount++
			}
		} else {
			// Still increment retry but don't mark as resolved
			updateQuery := `
				UPDATE dlq_messages
				SET retry_count = retry_count + 1, last_retry_at = NOW()
				WHERE message_id = $1
			`
			if _, err := db.Exec(updateQuery, messageID); err != nil {
				logger.Error("Error updating retry count for message %s: %v", messageID, err)
			}
		}

		retryCount++
	}

	if retryCount > 0 {
		logger.Info("✅ DLQ auto-retry completed: processed %d messages, %d resolved", retryCount, successCount)
	} else {
		logger.Info("✅ DLQ auto-retry completed: no messages to retry")
	}
}

// StopDLQAutoRetry stops the automatic DLQ retry mechanism
func StopDLQAutoRetry() {
	if dlqRetryTicker != nil {
		dlqRetryTicker.Stop()
	}
	if stopDLQRetry != nil {
		close(stopDLQRetry)
	}
	logger.Info("✅ DLQ auto-retry stopped")
}

// getDBConnection is a helper to get the database connection
// Returns the database connection from db package
func getDBConnection() *sql.DB {
	return db.DB
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
// On error, messages are sent to the DLQ
func handleKafkaMessage(msg kafka.Message) {
	_ = handleKafkaMessageForRetry(msg)
}

// handleKafkaMessageForRetry processes incoming Kafka messages and returns whether it was successful
// Returns true if message was processed successfully (not sent to DLQ)
// Returns false if message was sent to DLQ
func handleKafkaMessageForRetry(msg kafka.Message) bool {
	logger.Info("📨 Received message from topic: %s, partition: %d, offset: %d", msg.Topic, msg.Partition, msg.Offset)
	logger.Info("   Key: %s, Size: %d bytes", string(msg.Key), len(msg.Value))

	// Parse the message value as JSON
	var eventData map[string]interface{}
	err := json.Unmarshal(msg.Value, &eventData)
	if err != nil {
		logger.Error("Error unmarshaling message: %v", err)
		// Send to DLQ on unmarshal error
		_ = SendToDLQ(msg.Topic, string(msg.Key), msg.Value, "Failed to unmarshal JSON: "+err.Error())
		return false
	}

	logger.Info("   Event type: %v", eventData["event"])

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
	case "email.sent":
		handlerErr = handleEmailSent(eventData)
	case "application.submitted":
		handlerErr = handleApplicationSubmitted(eventData)
	case "interview.scheduled":
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
	logger.Info("✅ Successfully processed message (event: %s, key: %s)", eventType, string(msg.Key))
	return true
}

// ============================================================================
// EVENT HANDLERS - Process different event types
// ============================================================================

// handleLeadCreated processes lead.created events
func handleLeadCreated(event map[string]interface{}) error {
	logger.Info("🆕 Processing lead.created event:")
	logger.Info("   Student ID: %v", event["student_id"])
	logger.Info("   Name: %v", event["name"])
	logger.Info("   Email: %v", event["email"])
	logger.Info("   Counselor: %v", event["counselor_name"])
	return nil
}

// handlePaymentInitiated processes payment.initiated events
func handlePaymentInitiated(event map[string]interface{}) error {
	logger.Info("💳 Processing payment.initiated event:")
	logger.Info("   Student ID: %v", event["student_id"])
	logger.Info("   Order ID: %v", event["order_id"])
	logger.Info("   Amount: %v %v", event["amount"], event["currency"])
	logger.Info("   Payment Type: %v", event["payment_type"])
	return nil
}

// handlePaymentVerified processes payment.verified events
func handlePaymentVerified(event map[string]interface{}) error {
	logger.Info("✅ Processing payment.verified event:")
	logger.Info("   Student ID: %v", event["student_id"])
	logger.Info("   Payment ID: %v", event["payment_id"])
	logger.Info("   Status: %v", event["status"])
	return nil
}

// handleEmailSent processes email.sent events
func handleEmailSent(event map[string]interface{}) error {
	logger.Info("📧 Processing email.sent event:")
	logger.Info("   Recipient: %v", event["recipient"])
	logger.Info("   Subject: %v", event["subject"])
	return nil
}

// handleApplicationSubmitted processes application events
func handleApplicationSubmitted(event map[string]interface{}) error {
	logger.Info("📝 Processing application event:")
	logger.Info("   Student ID: %v", event["student_id"])
	logger.Info("   Course: %v", event["course"])
	logger.Info("   Status: %v", event["status"])
	return nil
}

// handleInterviewScheduled processes interview.scheduled events
func handleInterviewScheduled(event map[string]interface{}) error {
	logger.Info("📅 Processing interview.scheduled event:")

	email, ok := event["email"].(string)
	if !ok || email == "" {
		logger.Warn("Invalid email in interview.scheduled event")
		return fmt.Errorf("invalid email")
	}

	meetLink, ok := event["meet_link"].(string)
	if !ok || meetLink == "" {
		logger.Warn("Invalid meet_link in interview.scheduled event")
		return fmt.Errorf("invalid meet_link")
	}

	logger.Info("   Student ID: %v", event["student_id"])
	logger.Info("   Email: %v", email)
	logger.Info("   Meet Link: %v", meetLink)

	// Send email notification using existing SendEmail function
	subject := "Your Interview Meeting Link"
	body := "Your interview meeting has been scheduled.\n\nMeeting Link: " + meetLink

	return SendEmail(email, subject, body)
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
