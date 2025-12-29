package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
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

// AppLogger handles application logging to UI, console, and file
type AppLogger struct {
	dataBinding binding.StringList
	logFile     *os.File
	mu          sync.Mutex
}

// NewAppLogger creates a new logger instance
func NewAppLogger(data binding.StringList) *AppLogger {
	// Ensure logs dir exists
	os.MkdirAll("logs", 0755)
	
	// Open log file (append mode)
	logPath := filepath.Join("logs", "gamebot.log")
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Failed to open log file: %v\n", err)
	}

	return &AppLogger{
		dataBinding: data,
		logFile:     f,
	}
}

// Close closes the file handle
func (l *AppLogger) Close() {
	if l.logFile != nil {
		l.logFile.Close()
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

// Debug logs a debug message to stdout and file only (to keep UI clean)
func (l *AppLogger) Debug(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	fullMsg := fmt.Sprintf("[DEBUG] [%s] %s\n", timestamp, msg)
	
l.writeToConsoleAndFile(fullMsg)
}

// log handles the formatting and appending
func (l *AppLogger) log(level, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format("15:04:05") // UI uses short time
	uiMsg := fmt.Sprintf("[%s] %s: %s", timestamp, level, msg)

	// UI Update (Thread safe via binding)
	l.dataBinding.Append(uiMsg)
	
	// Keep log size manageable in UI
	list, _ := l.dataBinding.Get()
	if len(list) > 100 {
		l.dataBinding.Set(list[1:])
	}
	
	// File/Console Update
	fullTimestamp := time.Now().Format("2006-01-02 15:04:05")
	fileMsg := fmt.Sprintf("[%s] [%s] %s\n", level, fullTimestamp, msg)
	l.writeToConsoleAndFile(fileMsg)
}

func (l *AppLogger) writeToConsoleAndFile(msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	// Console
	fmt.Print(msg)
	
	// File
	if l.logFile != nil {
		if _, err := l.logFile.WriteString(msg); err != nil {
			fmt.Printf("Error writing to log file: %v\n", err)
		}
	}
}