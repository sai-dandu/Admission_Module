package main

import (
	"admission-module/config"
	"admission-module/db"
	"admission-module/http"
	"admission-module/logger"
	"admission-module/services"
	"fmt"
	"log"
	netHttp "net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

func main() {
	// Determine project root by searching upward for go.mod
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal("Error getting current working directory:", err)
	}

	absProjectRoot := findProjectRoot(cwd)
	if absProjectRoot == "" {
		log.Fatalf("Could not locate project root (go.mod) from %s", cwd)
	}

	if err := os.Chdir(absProjectRoot); err != nil {
		log.Fatal("Error changing to project root:", err)
	}

	// Load configuration
	config.LoadConfig()

	// Initialize Kafka producer (non-fatal)
	services.InitProducer()

	// Initialize Kafka DLQ producer (non-fatal)
	services.InitDLQProducer()

	// Initialize and start Kafka consumer (non-fatal)
	consumerTopics := []string{"payments", "applications", "emails"}
	if err := services.InitConsumer(consumerTopics); err != nil {
		logger.Warn("Failed to initialize Kafka consumer: %v", err)
	} else {
		services.StartConsumer()
	}

	// Start DLQ auto-retry mechanism
	services.StartDLQAutoRetry()

	// Initialize database
	if err := db.InitDB(); err != nil {
		logger.Fatal("Error initializing database: %v", err)
	}

	// Register email processor for Kafka consumer
	// This callback will be invoked when Kafka consumer receives email.send events
	services.RegisterEmailProcessor(func(event map[string]interface{}) error {
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
		return services.SendEmailDirect(recipient, subject, body, attachment...)
	})

	// Register interview scheduler for Kafka consumer
	// This callback will be invoked when Kafka consumer receives interview.schedule events
	services.RegisterInterviewScheduler(func(studentID int, email string) error {
		_, err := services.ScheduleMeet(studentID, email)
		return err
	})

	// Setup routes
	http.SetupRoutes()

	// Set up graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start server in a goroutine
	go func() {
		log.Fatal(netHttp.ListenAndServe(":8080", nil))
	}()

	// Wait for shutdown signal
	<-sigChan

	// Stop DLQ auto-retry
	services.StopDLQAutoRetry()

	// Stop consumer gracefully
	if err := services.StopConsumer(); err != nil {
		logger.Error("Error stopping Kafka consumer: %v", err)
	}

	// Close Kafka producer gracefully
	if err := services.Close(); err != nil {
		logger.Error("Error closing Kafka producer: %v", err)
	}
}

// findProjectRoot walks up from start and returns the first directory containing go.mod
func findProjectRoot(start string) string {
	dir := start
	for {
		// check for go.mod
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		// move up
		parent := filepath.Dir(dir)
		if parent == dir || strings.HasSuffix(dir, ":\\") || parent == "" {
			break
		}
		dir = parent
	}
	return ""
}
