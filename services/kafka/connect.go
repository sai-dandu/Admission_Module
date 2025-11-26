package kafka

import (
	"admission-module/config"
	"admission-module/db"
	"admission-module/logger"
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
)

var (
	dlqProducer    *kafka.Writer
	dlqMutex       sync.Mutex
	dlqRetryTicker *time.Ticker
	stopDLQRetry   chan bool
)

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

	// If DLQ producer is available, attempt a single publish and on failure
	// fall back to storing the message in the DB. If the broker reports the
	// topic/partition does not exist, disable the DLQ producer so we don't
	// repeatedly attempt Kafka publishes for the same configuration.
	if dlqProducer != nil && config.AppConfig.KafkaDLQTopic != "" {
		dlqMessage := map[string]interface{}{
			"original_topic": topic,
			"original_key":   key,
			"original_value": string(value),
			"error_message":  errorMsg,
			"timestamp":      time.Now().Unix(),
			"failure_reason": "Processing failed",
		}

		dlqPayload, err := json.Marshal(dlqMessage)
		if err == nil {
			msg := kafka.Message{Key: []byte(key), Value: dlqPayload}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err = dlqProducer.WriteMessages(ctx, msg)
			cancel()
			if err == nil {
				logger.Info("Message sent to DLQ topic: %s", config.AppConfig.KafkaDLQTopic)
				// also store in DB for persistence
				_ = StoreDLQMessage(topic, key, value, errorMsg)
				return nil
			}

			// If topic is missing on broker, disable DLQ producer and fallback
			if strings.Contains(err.Error(), "Unknown Topic Or Partition") || strings.Contains(strings.ToLower(err.Error()), "unknown topic") {
				logger.Warn("DLQ topic missing on broker; disabling DLQ producer: %v", err)
				dlqProducer = nil
				// fallthrough to store in DB
			} else {
				// other errors - log and fall back to DB as well
				logger.Warn("DLQ publish failed, storing to DB: %v", err)
			}
		} else {
			logger.Error("Error marshaling DLQ message: %v", err)
		}
	}

	// Always store in database for persistence
	return StoreDLQMessage(topic, key, value, errorMsg)
}

// StoreDLQMessage stores a failed message in the database
func StoreDLQMessage(topic, key string, value []byte, errorMsg string) error {
	// Get database connection from your db package
	// This is a placeholder - adjust based on your actual db setup
	dbConn := getDBConnection()
	if dbConn == nil {
		logger.Warn("Database connection not available for DLQ storage")
		return nil
	}

	query := `
		INSERT INTO dlq_messages (message_id, topic, key, value, error_message, created_at)
		VALUES (gen_random_uuid(), $1, $2, $3::jsonb, $4, NOW())
		ON CONFLICT (message_id) DO NOTHING
	`

	// Pass value as []byte directly - PostgreSQL will handle JSONB conversion
	_, err := dbConn.Exec(query, topic, key, value, errorMsg)
	if err != nil {
		logger.Error("Error storing DLQ message in database: %v", err)
		return err
	}

	logger.Info("✅ DLQ message stored in database. Topic: %s, Key: %s", topic, key)
	return nil
}

// GetDLQMessages retrieves unresolved DLQ messages from database
func GetDLQMessages(limit int) ([]map[string]interface{}, error) {
	dbConn := getDBConnection()
	if dbConn == nil {
		return nil, nil
	}

	query := `
		SELECT id, message_id, topic, key, value, error_message, retry_count, created_at
		FROM dlq_messages
		WHERE resolved = FALSE
		ORDER BY created_at DESC
		LIMIT $1
	`

	rows, err := dbConn.Query(query, limit)
	if err != nil {
		logger.Error("Error querying DLQ messages: %v", err)
		return nil, err
	}
	defer rows.Close()

	var messages []map[string]interface{}
	for rows.Next() {
		var id int
		var messageID, topic, key string
		var value []byte
		var errorMsg string
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
	dbConn := getDBConnection()
	if dbConn == nil {
		return nil
	}

	query := `
		SELECT value, topic, key FROM dlq_messages WHERE message_id = $1
	`

	var value []byte
	var topic, key string
	err := dbConn.QueryRow(query, messageID).Scan(&value, &topic, &key)
	if err != nil {
		logger.Error("Error retrieving DLQ message for retry: %v", err)
		return err
	}

	// Attempt to reprocess
	var eventData map[string]interface{}
	if err := json.Unmarshal(value, &eventData); err != nil {
		logger.Error("Error unmarshaling DLQ message for retry: %v", err)
		return err
	}

	// Reprocess the message and check if successful
	wasSuccessful := HandleKafkaMessageForRetry(kafka.Message{
		Topic: topic,
		Key:   []byte(key),
		Value: value,
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
	_, err = dbConn.Exec(updateQuery, messageID)
	return err
}

// ResolveDLQMessage marks a DLQ message as resolved
func ResolveDLQMessage(messageID string, notes string) error {
	dbConn := getDBConnection()
	if dbConn == nil {
		return nil
	}

	query := `
		UPDATE dlq_messages
		SET resolved = TRUE, resolved_at = NOW(), notes = $2
		WHERE message_id = $1
	`

	_, err := dbConn.Exec(query, messageID, notes)
	if err != nil {
		logger.Error("Error resolving DLQ message: %v", err)
		return err
	}

	logger.Info("✅ DLQ message %s marked as resolved", messageID)
	return nil
}

// GetDLQStats retrieves statistics about DLQ messages
func GetDLQStats() (map[string]interface{}, error) {
	dbConn := getDBConnection()
	if dbConn == nil {
		return nil, nil
	}

	var totalMessages, unresolvedMessages, resolvedMessages int

	// Get total DLQ messages
	err := dbConn.QueryRow("SELECT COUNT(*) FROM dlq_messages").Scan(&totalMessages)
	if err != nil {
		logger.Error("Error getting total DLQ messages count: %v", err)
		return nil, err
	}

	// Get unresolved DLQ messages
	err = dbConn.QueryRow("SELECT COUNT(*) FROM dlq_messages WHERE resolved = FALSE").Scan(&unresolvedMessages)
	if err != nil {
		logger.Error("Error getting unresolved DLQ messages count: %v", err)
		return nil, err
	}

	// Get resolved DLQ messages
	err = dbConn.QueryRow("SELECT COUNT(*) FROM dlq_messages WHERE resolved = TRUE").Scan(&resolvedMessages)
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
	dbConn := getDBConnection()
	if dbConn == nil {
		logger.Warn("Database connection not available for DLQ auto-retry")
		return
	}

	query := `
		SELECT message_id, value, topic, key, retry_count, max_retries
		FROM dlq_messages
		WHERE resolved = FALSE AND retry_count < max_retries
		ORDER BY created_at ASC
		LIMIT 10
	`

	rows, err := dbConn.Query(query)
	if err != nil {
		logger.Error("Error querying unresolved DLQ messages for retry: %v", err)
		return
	}
	defer rows.Close()

	retryCount := 0
	successCount := 0
	for rows.Next() {
		var messageID string
		var value []byte
		var topic, key string
		var retryAttempts, maxRetries int

		if err := rows.Scan(&messageID, &value, &topic, &key, &retryAttempts, &maxRetries); err != nil {
			logger.Error("Error scanning DLQ message for retry: %v", err)
			continue
		}

		logger.Info("🔄 Auto-retrying DLQ message %s (attempt %d/%d)", messageID, retryAttempts+1, maxRetries)

		// Attempt to reprocess the message - capture success/failure
		// We'll treat successful reprocessing (no SendToDLQ call) as success
		wasReprocessed := HandleKafkaMessageForRetry(kafka.Message{
			Topic: topic,
			Key:   []byte(key),
			Value: value,
		})

		logger.Info("🔍 Reprocessing result for message %s: success=%v", messageID, wasReprocessed)

		// Update retry count and mark as resolved if successful
		if wasReprocessed {
			updateQuery := `
				UPDATE dlq_messages
				SET retry_count = retry_count + 1, last_retry_at = NOW(), resolved = TRUE, resolved_at = NOW(), notes = 'Auto-retried successfully'
				WHERE message_id = $1
			`
			if _, err := dbConn.Exec(updateQuery, messageID); err != nil {
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
			if _, err := dbConn.Exec(updateQuery, messageID); err != nil {
				logger.Error("Error updating retry count for message %s: %v", messageID, err)
			}
		}

		retryCount++
	}

	if retryCount > 0 {
		logger.Info("✅ DLQ auto-retry completed: processed %d messages, %d resolved", retryCount, successCount)
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
