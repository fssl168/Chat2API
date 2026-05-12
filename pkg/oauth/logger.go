package oauth

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

var logLevelNames = map[LogLevel]string{
	LogLevelDebug: "DEBUG",
	LogLevelInfo:  "INFO",
	LogLevelWarn:  "WARN",
	LogLevelError: "ERROR",
}

var logLevelColors = map[LogLevel]string{
	LogLevelDebug: "\033[36m",
	LogLevelInfo:  "\033[32m",
	LogLevelWarn:  "\033[33m",
	LogLevelError: "\033[31m",
}

const colorReset = "\033[0m"

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp time.Time   `json:"timestamp"`
	Level     LogLevel    `json:"level"`
	Message   string      `json:"message"`
	Data      interface{} `json:"data,omitempty"`
	Duration  string      `json:"duration,omitempty"`
}

// FlowLogger provides structured logging for OAuth flow
type FlowLogger struct {
	sessionID string
	entries   []LogEntry
	mu        sync.RWMutex
	callbacks []ProgressCallback
}

// NewFlowLogger creates a new flow logger with session ID
func NewFlowLogger(sessionID string) *FlowLogger {
	return &FlowLogger{
		sessionID: sessionID,
		entries:   make([]LogEntry, 0),
		callbacks: make([]ProgressCallback, 0),
	}
}

// AddCallback adds a progress callback to receive log events
func (l *FlowLogger) AddCallback(cb ProgressCallback) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.callbacks = append(l.callbacks, cb)
}

// log adds a new log entry
func (l *FlowLogger) log(level LogLevel, message string, data interface{}) {
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
		Data:      data,
	}

	l.mu.Lock()
	l.entries = append(l.entries, entry)
	l.mu.Unlock()

	formatted := formatLogEntry(entry)
	log.Print(formatted)

	event := OAuthProgressEvent{
		Status:  convertLogLevelToStatus(level),
		Message: message,
		Data:    dataToMap(data),
	}

	for _, cb := range l.callbacks {
		cb(event)
	}
}

// Debug logs debug level message
func (l *FlowLogger) Debug(message string, data ...interface{}) {
	l.log(LogLevelDebug, message, packData(data...))
}

// Info logs info level message
func (l *FlowLogger) Info(message string, data ...interface{}) {
	l.log(LogLevelInfo, message, packData(data...))
}

// Warn logs warning level message
func (l *FlowLogger) Warn(message string, data ...interface{}) {
	l.log(LogLevelWarn, message, packData(data...))
}

// Error logs error level message
func (l *FlowLogger) Error(message string, data ...interface{}) {
	l.log(LogLevelError, message, packData(data...))
}

// Step logs a step with timing
func (l *FlowLogger) Step(stepNum int, message string, data ...interface{}) {
	packed := packData(data...)
	if packed == nil {
		packed = map[string]interface{}{"step": stepNum}
	} else if m, ok := packed.(map[string]interface{}); ok {
		m["step"] = stepNum
	}
	l.log(LogLevelInfo, message, packed)
}

// GetEntries returns all log entries
func (l *FlowLogger) GetEntries() []LogEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return append([]LogEntry(nil), l.entries...)
}

// GetJSON returns all entries as JSON
func (l *FlowLogger) GetJSON() ([]byte, error) {
	entries := l.GetEntries()
	return json.Marshal(entries)
}

// Clear clears all entries
func (l *FlowLogger) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = nil
}

// TimedAction executes an action and logs duration
func (l *FlowLogger) TimedAction(name string, fn func() error) error {
	start := time.Now()
	l.Info(fmt.Sprintf("⏱️ Starting: %s", name), "action", name)

	err := fn()

	duration := time.Since(start)
	if err != nil {
		l.Error(fmt.Sprintf("❌ Failed: %s (%v)", name, err),
			"action", name,
			"duration", duration.String(),
			"error", err.Error())
	} else {
		l.Info(fmt.Sprintf("✅ Completed: %s (%s)", name, duration.Round(time.Millisecond)),
			"action", name,
			"duration", duration.String())
	}

	return err
}

// formatLogEntry formats a log entry for console output
func formatLogEntry(entry LogEntry) string {
	color := logLevelColors[entry.Level]
	levelName := logLevelNames[entry.Level]
	timestamp := entry.Timestamp.Format("15:04:05.000")

	msg := fmt.Sprintf("%s[%s] [%-5s] %s%s%s",
		color, timestamp, levelName, entry.Message, colorReset, "")

	if entry.Data != nil {
		dataStr := formatData(entry.Data)
		if dataStr != "" {
			msg += " | " + dataStr
		}
	}

	return msg
}

// formatData formats data as string
func formatData(data interface{}) string {
	if data == nil {
		return ""
	}
	switch d := data.(type) {
	case map[string]interface{}:
		parts := make([]string, 0, len(d))
		for k, v := range d {
			parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		}
		return joinStrings(parts, ", ")
	default:
		return fmt.Sprintf("%v", data)
	}
}

func joinStrings(strs []string, sep string) string {
	result := ""
	for i, s := range strs {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}

func packData(data ...interface{}) interface{} {
	if len(data) == 0 {
		return nil
	}
	if len(data) == 1 {
		return data[0]
	}
	m := make(map[string]interface{})
	for i := 0; i < len(data); i += 2 {
		key, _ := data[i].(string)
		var val interface{}
		if i+1 < len(data) {
			val = data[i+1]
		} else {
			val = ""
		}
		m[key] = val
	}
	return m
}

func dataToMap(data interface{}) map[string]interface{} {
	if data == nil {
		return nil
	}
	if m, ok := data.(map[string]interface{}); ok {
		return m
	}
	return map[string]interface{}{"data": data}
}

func convertLogLevelToStatus(level LogLevel) OAuthStatus {
	switch level {
	case LogLevelError:
		return OAuthStatusError
	case LogLevelWarn:
		return OAuthStatusPending
	case LogLevelInfo:
		return OAuthStatusSuccess
	default:
		return OAuthStatusIdle
	}
}

// Global logger instance
var globalLogger *FlowLogger

// SetGlobalLogger sets the global flow logger
func SetGlobalLogger(logger *FlowLogger) {
	globalLogger = logger
}

// GetGlobalLogger returns the global flow logger
func GetGlobalLogger() *FlowLogger {
	if globalLogger == nil {
		globalLogger = NewFlowLogger("default")
	}
	return globalLogger
}
