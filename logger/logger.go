package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// Level represents the severity level of a log entry
type Level int

const (
	// DEBUG level for detailed debugging information
	DEBUG Level = iota
	// INFO level for general information
	INFO
	// WARN level for warning messages
	WARN
	// ERROR level for error messages
	ERROR
	// FATAL level for fatal errors that cause program exit
	FATAL
)

// String returns the string representation of the log level
func (l Level) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	case FATAL:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// Logger represents a structured logger
type Logger struct {
	level  Level
	logger *log.Logger
	writer io.Writer
}

// Config holds the configuration for the logger
type Config struct {
	Level        Level
	Output       io.Writer
	TimeFormat   string
	EnableCaller bool
}

// New creates a new logger with the given configuration
func New(config Config) *Logger {
	if config.Output == nil {
		config.Output = os.Stdout
	}
	if config.TimeFormat == "" {
		config.TimeFormat = "2006-01-02 15:04:05"
	}

	logger := &Logger{
		level:  config.Level,
		writer: config.Output,
	}

	logger.logger = log.New(logger.writer, "", 0)

	return logger
}

// NewDefault creates a logger with default configuration
func NewDefault() *Logger {
	return New(Config{
		Level:        INFO,
		Output:       os.Stdout,
		TimeFormat:   "2006-01-02 15:04:05",
		EnableCaller: false,
	})
}

// SetLevel sets the minimum log level
func (l *Logger) SetLevel(level Level) {
	l.level = level
}

// log writes a log entry if the level is enabled
func (l *Logger) log(level Level, message string, args ...interface{}) {
	if level < l.level {
		return
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	levelStr := level.String()

	var caller string
	if l.logger.Flags()&log.Lshortfile != 0 {
		_, file, line, ok := runtime.Caller(2)
		if ok {
			caller = fmt.Sprintf("%s:%d ", filepath.Base(file), line)
		}
	}

	formattedMessage := message
	if len(args) > 0 {
		formattedMessage = fmt.Sprintf(message, args...)
	}

	logEntry := fmt.Sprintf("[%s] %s %s%s\n", timestamp, levelStr, caller, formattedMessage)

	l.logger.Print(logEntry)

	if level == FATAL {
		os.Exit(1)
	}
}

// Debug logs a debug message
func (l *Logger) Debug(message string, args ...interface{}) {
	l.log(DEBUG, message, args...)
}

// Info logs an info message
func (l *Logger) Info(message string, args ...interface{}) {
	l.log(INFO, message, args...)
}

// Warn logs a warning message
func (l *Logger) Warn(message string, args ...interface{}) {
	l.log(WARN, message, args...)
}

// Error logs an error message
func (l *Logger) Error(message string, args ...interface{}) {
	l.log(ERROR, message, args...)
}

// Fatal logs a fatal message and exits the program
func (l *Logger) Fatal(message string, args ...interface{}) {
	l.log(FATAL, message, args...)
}

// WithCaller enables caller information in log entries
func (l *Logger) WithCaller() *Logger {
	l.logger.SetFlags(log.Lshortfile)
	return l
}

// WithFields creates a logger with structured fields (simple implementation)
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	// For simplicity, we'll just prefix the message with fields
	// In a real implementation, you might want to use a more sophisticated approach
	prefix := ""
	for k, v := range fields {
		prefix += fmt.Sprintf("%s=%v ", k, v)
	}
	if prefix != "" {
		prefix = prefix[:len(prefix)-1] + " " // remove last space and add space after
	}

	return &Logger{
		level:  l.level,
		writer: l.writer,
		logger: log.New(l.writer, prefix, l.logger.Flags()),
	}
}

// Global logger instance
var defaultLogger *Logger

// init initializes the default logger
func init() {
	defaultLogger = NewDefault()
}

// SetDefault sets the default logger
func SetDefault(logger *Logger) {
	defaultLogger = logger
}

// Debug logs a debug message using the default logger
func Debug(message string, args ...interface{}) {
	defaultLogger.Debug(message, args...)
}

// Info logs an info message using the default logger
func Info(message string, args ...interface{}) {
	defaultLogger.Info(message, args...)
}

// Warn logs a warning message using the default logger
func Warn(message string, args ...interface{}) {
	defaultLogger.Warn(message, args...)
}

// Error logs an error message using the default logger
func Error(message string, args ...interface{}) {
	defaultLogger.Error(message, args...)
}

// Fatal logs a fatal message using the default logger
func Fatal(message string, args ...interface{}) {
	defaultLogger.Fatal(message, args...)
}
