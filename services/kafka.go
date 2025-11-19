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

	producer = &kafka.Writer{
		Addr:     kafka.TCP(validBrokers...),
		Balancer: &kafka.LeastBytes{},
		Async:    false,
		// Set a reasonable write timeout
		WriteTimeout: 10 * time.Second,
		// Allow up to 10 retries
		RequiredAcks: kafka.RequireAll,
	}

	logger.Info("Kafka producer initialized. Brokers=%v", validBrokers)
	isConnected = true
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

// Close gracefully closes the Kafka producer
func Close() error {
	producerMutex.Lock()
	defer producerMutex.Unlock()

	if producer != nil {
		return producer.Close()
	}
	return nil
}
