package logger

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2/data/binding"
)

// LogLevel defines the severity of the log
type LogLevel int

const (
	LevelInfo LogLevel = iota
	LevelError
	LevelDebug
)

// AppLogger handles application logging to UI and console
type AppLogger struct {
	dataBinding binding.StringList
}

// NewAppLogger creates a new logger instance
func NewAppLogger(data binding.StringList) *AppLogger {
	return &AppLogger{
		dataBinding: data,
	}
}

// Info logs an informational message
func (l *AppLogger) Info(format string, args ...interface{}) {
	l.log("INFO", format, args...)
}

// Error logs an error message
func (l *AppLogger) Error(format string, args ...interface{}) {
	l.log("ERROR", format, args...)
}

// Debug logs a debug message to stdout only (to keep UI clean)
func (l *AppLogger) Debug(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format("15:04:05")
	fmt.Printf("[DEBUG] [%s] %s\n", timestamp, msg)
}

// log handles the formatting and appending
func (l *AppLogger) log(level, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format("15:04:05")
	formattedMsg := fmt.Sprintf("[%s] %s: %s", timestamp, level, msg)

	// Append to data binding
	l.dataBinding.Append(formattedMsg)

	// Optional: Keep log size manageable (e.g., last 100 lines)
	list, _ := l.dataBinding.Get()
	if len(list) > 100 {
		l.dataBinding.Set(list[1:])
	}
}
