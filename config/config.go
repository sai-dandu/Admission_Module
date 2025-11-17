package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string

	RazorpayKeyID     string
	RazorpayKeySecret string

	SMTPHost  string
	SMTPPort  string
	SMTPUser  string
	SMTPPass  string
	EmailFrom string
	// Kafka
	KafkaBrokers string
	KafkaTopic   string
}

var AppConfig Config

func LoadConfig() {
	// Try loading .env from different locations
	envLocations := []string{
		".env",              // project root
		"config/.env",       // config subdirectory
		"../config/.env",    // one level up
		"../../config/.env", // two levels up
	}

	envLoaded := false
	for _, location := range envLocations {
		if err := godotenv.Load(location); err == nil {
			envLoaded = true
			break
		}
	}

	if !envLoaded {
		log.Println("No .env file found, using environment variables")
	}

	AppConfig = Config{
		DBHost:     getEnvWithDefault("DB_HOST", "localhost"),
		DBPort:     getEnvWithDefault("DB_PORT", "5432"),
		DBUser:     getEnvWithDefault("DB_USER", "postgres"),
		DBPassword: getEnvWithDefault("DB_PASSWORD", "Sai@6303179072$"),
		DBName:     getEnvWithDefault("DB_NAME", "postgres"),

		RazorpayKeyID:     os.Getenv("RazorpayKeyID"),
		RazorpayKeySecret: os.Getenv("RazorpayKeySecret"),

		SMTPHost:  getEnvWithDefault("SMTP_HOST", "smtp.gmail.com"),
		SMTPPort:  getEnvWithDefault("SMTP_PORT", "587"),
		SMTPUser:  os.Getenv("SMTP_USER"),
		SMTPPass:  os.Getenv("SMTP_PASS"),
		EmailFrom: os.Getenv("EMAIL_FROM"),

		// Kafka settings (comma-separated brokers)
		KafkaBrokers: getEnvWithDefault("KAFKA_BROKERS", "127.0.0.1:9092"),
		KafkaTopic:   getEnvWithDefault("KAFKA_TOPIC", "admissions.payments"),
	}
}

func getEnvWithDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func GetDBConnString() string {
	return "host=" + AppConfig.DBHost +
		" port=" + AppConfig.DBPort +
		" user=" + AppConfig.DBUser +
		" password=" + AppConfig.DBPassword +
		" dbname=" + AppConfig.DBName +
		" sslmode=disable"
}
