package kafka

import (
	"admission-module/config"
	"admission-module/logger"
	"context"
	"encoding/json"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
)

var (
	producer      *kafka.Writer
	producerMutex sync.Mutex
	isConnected   bool
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
			// include configured DLQ topic if present
			if t := strings.TrimSpace(config.AppConfig.KafkaDLQTopic); t != "" {
				// avoid duplicates
				found := false
				for _, rt := range requiredTopics {
					if rt == t {
						found = true
						break
					}
				}
				if !found {
					requiredTopics = append(requiredTopics, t)
				}
			}
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
					successCount++
				}
			}

			conn.Close()

			// If we created or found all topics, we're done
			if successCount >= len(requiredTopics) {
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
// If Kafka is disabled or not initialized, returns nil (best-effort)
func Publish(topic, key string, value interface{}) error {
	producerMutex.Lock()
	if producer == nil && config.AppConfig.KafkaBrokers != "" {
		producerMutex.Unlock()
		InitProducer()
		producerMutex.Lock()
	}
	defer producerMutex.Unlock()

	// If Kafka is disabled or not initialized, skip publishing silently (best-effort)
	if producer == nil || config.AppConfig.KafkaBrokers == "" {
		return nil
	}

	payload, err := json.Marshal(value)
	if err != nil {
		logger.Error("Error marshaling Kafka message: %v", err)
		return err
	}

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
			isConnected = true
			return nil
		}

		lastErr = err
		logger.Warn("Kafka publish attempt %d failed: %v", attempt+1, err)

		if attempt < 2 {
			backoffTime := time.Duration(math.Pow(2, float64(attempt))) * time.Second
			time.Sleep(backoffTime)
		}
		isConnected = false

		// If this is the second attempt failing, try to recreate the producer
		// to avoid stale broker metadata
		if attempt == 1 {
			logger.Info("Attempting to recreate Kafka producer due to connection issues")
			if producer != nil {
				producer.Close()
			}
			InitProducer()
		}
	}

	// Send to DLQ if all retries failed (database only, avoid recursion)
	logger.Info("Sending failed message to DLQ. Topic: %s, Key: %s", topic, key)
	if dlqErr := StoreDLQMessage(topic, key, payload, lastErr.Error()); dlqErr != nil {
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

// Close gracefully closes the Kafka producer
func Close() error {
	producerMutex.Lock()
	defer producerMutex.Unlock()

	if producer != nil {
		return producer.Close()
	}
	return nil
}
