package services

import (
	"admission-module/services/kafka"
)

func InitProducer() {
	kafka.InitProducer()
}

func Publish(topic, key string, value interface{}) error {
	return kafka.Publish(topic, key, value)
}

func IsConnected() bool {
	return kafka.IsConnected()
}

func Close() error {
	return kafka.Close()
}

func InitConsumer(topics []string) error {
	return kafka.InitConsumer(topics)
}

func StartConsumer() {
	kafka.StartConsumer()
}

func StopConsumer() error {
	return kafka.StopConsumer()
}

func IsConsumerRunning() bool {
	return kafka.IsConsumerRunning()
}

func RegisterEmailProcessor(fn func(map[string]interface{}) error) {
	kafka.RegisterEmailProcessor(fn)
}

func RegisterInterviewScheduler(fn func(int, string) error) {
	kafka.RegisterInterviewScheduler(fn)
}

func InitDLQProducer() {
	kafka.InitDLQProducer()
}

func SendToDLQ(topic, key string, value []byte, errorMsg string) error {
	return kafka.SendToDLQ(topic, key, value, errorMsg)
}

func StoreDLQMessage(topic, key string, value []byte, errorMsg string) error {
	return kafka.StoreDLQMessage(topic, key, value, errorMsg)
}

func GetDLQMessages(limit int) ([]map[string]interface{}, error) {
	return kafka.GetDLQMessages(limit)
}

func RetryDLQMessage(messageID string) error {
	return kafka.RetryDLQMessage(messageID)
}

func ResolveDLQMessage(messageID string, notes string) error {
	return kafka.ResolveDLQMessage(messageID, notes)
}

func GetDLQStats() (map[string]interface{}, error) {
	return kafka.GetDLQStats()
}

func StartDLQAutoRetry() {
	kafka.StartDLQAutoRetry()
}

func StopDLQAutoRetry() {
	kafka.StopDLQAutoRetry()
}
