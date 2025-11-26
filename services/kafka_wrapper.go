package services

import (
	"admission-module/services/kafka"
)

// ============================================================================
// KAFKA PRODUCER WRAPPER - Re-exports from kafka package
// ============================================================================

// InitProducer initializes a Kafka writer using brokers from the config
func InitProducer() {
	kafka.InitProducer()
}

// Publish marshals value to JSON and publishes to the given topic with key
func Publish(topic, key string, value interface{}) error {
	return kafka.Publish(topic, key, value)
}

// IsConnected returns true if Kafka producer is connected and ready
func IsConnected() bool {
	return kafka.IsConnected()
}

// Close gracefully closes the Kafka producer
func Close() error {
	return kafka.Close()
}

// ============================================================================
// KAFKA CONSUMER WRAPPER - Re-exports from kafka package
// ============================================================================

// InitConsumer initializes a Kafka reader (consumer) for specified topics
func InitConsumer(topics []string) error {
	return kafka.InitConsumer(topics)
}

// StartConsumer starts consuming messages in a separate goroutine
func StartConsumer() {
	kafka.StartConsumer()
}

// StopConsumer stops the consumer gracefully
func StopConsumer() error {
	return kafka.StopConsumer()
}

// IsConsumerRunning returns true if the consumer is actively running
func IsConsumerRunning() bool {
	return kafka.IsConsumerRunning()
}

// ============================================================================
// KAFKA DLQ WRAPPER - Re-exports from kafka package
// ============================================================================

// InitDLQProducer initializes a Kafka writer for the DLQ topic
func InitDLQProducer() {
	kafka.InitDLQProducer()
}

// SendToDLQ publishes a failed message to the Dead Letter Queue
func SendToDLQ(topic, key string, value []byte, errorMsg string) error {
	return kafka.SendToDLQ(topic, key, value, errorMsg)
}

// StoreDLQMessage stores a failed message in the database
func StoreDLQMessage(topic, key string, value []byte, errorMsg string) error {
	return kafka.StoreDLQMessage(topic, key, value, errorMsg)
}

// GetDLQMessages retrieves unresolved DLQ messages from database
func GetDLQMessages(limit int) ([]map[string]interface{}, error) {
	return kafka.GetDLQMessages(limit)
}

// RetryDLQMessage attempts to reprocess a DLQ message
func RetryDLQMessage(messageID string) error {
	return kafka.RetryDLQMessage(messageID)
}

// ResolveDLQMessage marks a DLQ message as resolved
func ResolveDLQMessage(messageID string, notes string) error {
	return kafka.ResolveDLQMessage(messageID, notes)
}

// GetDLQStats retrieves statistics about DLQ messages
func GetDLQStats() (map[string]interface{}, error) {
	return kafka.GetDLQStats()
}

// StartDLQAutoRetry starts a background goroutine that automatically retries failed DLQ messages
func StartDLQAutoRetry() {
	kafka.StartDLQAutoRetry()
}

// StopDLQAutoRetry stops the automatic DLQ retry mechanism
func StopDLQAutoRetry() {
	kafka.StopDLQAutoRetry()
}
