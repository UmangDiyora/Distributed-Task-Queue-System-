package monitoring

import (
	"fmt"
	"io"
	"log"
	"os"
	"time"
)

// LogLevel represents the logging level
type LogLevel int

const (
	// LogLevelDebug for debug messages
	LogLevelDebug LogLevel = iota
	// LogLevelInfo for informational messages
	LogLevelInfo
	// LogLevelWarn for warning messages
	LogLevelWarn
	// LogLevelError for error messages
	LogLevelError
)

// String returns the string representation of the log level
func (l LogLevel) String() string {
	switch l {
	case LogLevelDebug:
		return "DEBUG"
	case LogLevelInfo:
		return "INFO"
	case LogLevelWarn:
		return "WARN"
	case LogLevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Logger provides structured logging functionality
type Logger struct {
	level      LogLevel
	output     io.Writer
	prefix     string
	fields     map[string]interface{}
}

// NewLogger creates a new logger instance
func NewLogger(level LogLevel, output io.Writer) *Logger {
	if output == nil {
		output = os.Stdout
	}

	return &Logger{
		level:  level,
		output: output,
		fields: make(map[string]interface{}),
	}
}

// WithField adds a field to the logger context
func (l *Logger) WithField(key string, value interface{}) *Logger {
	newLogger := &Logger{
		level:  l.level,
		output: l.output,
		prefix: l.prefix,
		fields: make(map[string]interface{}),
	}

	for k, v := range l.fields {
		newLogger.fields[k] = v
	}
	newLogger.fields[key] = value

	return newLogger
}

// WithFields adds multiple fields to the logger context
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	newLogger := &Logger{
		level:  l.level,
		output: l.output,
		prefix: l.prefix,
		fields: make(map[string]interface{}),
	}

	for k, v := range l.fields {
		newLogger.fields[k] = v
	}
	for k, v := range fields {
		newLogger.fields[k] = v
	}

	return newLogger
}

// Debug logs a debug message
func (l *Logger) Debug(msg string) {
	l.log(LogLevelDebug, msg)
}

// Debugf logs a formatted debug message
func (l *Logger) Debugf(format string, args ...interface{}) {
	l.log(LogLevelDebug, fmt.Sprintf(format, args...))
}

// Info logs an informational message
func (l *Logger) Info(msg string) {
	l.log(LogLevelInfo, msg)
}

// Infof logs a formatted informational message
func (l *Logger) Infof(format string, args ...interface{}) {
	l.log(LogLevelInfo, fmt.Sprintf(format, args...))
}

// Warn logs a warning message
func (l *Logger) Warn(msg string) {
	l.log(LogLevelWarn, msg)
}

// Warnf logs a formatted warning message
func (l *Logger) Warnf(format string, args ...interface{}) {
	l.log(LogLevelWarn, fmt.Sprintf(format, args...))
}

// Error logs an error message
func (l *Logger) Error(msg string) {
	l.log(LogLevelError, msg)
}

// Errorf logs a formatted error message
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.log(LogLevelError, fmt.Sprintf(format, args...))
}

// log writes a log message
func (l *Logger) log(level LogLevel, msg string) {
	if level < l.level {
		return
	}

	timestamp := time.Now().Format(time.RFC3339)
	levelStr := level.String()

	// Build fields string
	fieldsStr := ""
	if len(l.fields) > 0 {
		fieldsStr = " "
		for k, v := range l.fields {
			fieldsStr += fmt.Sprintf("%s=%v ", k, v)
		}
	}

	logMsg := fmt.Sprintf("[%s] %s: %s%s\n", timestamp, levelStr, msg, fieldsStr)

	if l.prefix != "" {
		logMsg = l.prefix + " " + logMsg
	}

	l.output.Write([]byte(logMsg))
}

// SetLevel sets the logging level
func (l *Logger) SetLevel(level LogLevel) {
	l.level = level
}

// GetLevel returns the current logging level
func (l *Logger) GetLevel() LogLevel {
	return l.level
}

// Default logger instance
var defaultLogger = NewLogger(LogLevelInfo, os.Stdout)

// SetDefaultLogger sets the default logger
func SetDefaultLogger(logger *Logger) {
	defaultLogger = logger
}

// GetDefaultLogger returns the default logger
func GetDefaultLogger() *Logger {
	return defaultLogger
}

// Debug logs a debug message using the default logger
func Debug(msg string) {
	defaultLogger.Debug(msg)
}

// Debugf logs a formatted debug message using the default logger
func Debugf(format string, args ...interface{}) {
	defaultLogger.Debugf(format, args...)
}

// Info logs an informational message using the default logger
func Info(msg string) {
	defaultLogger.Info(msg)
}

// Infof logs a formatted informational message using the default logger
func Infof(format string, args ...interface{}) {
	defaultLogger.Infof(format, args...)
}

// Warn logs a warning message using the default logger
func Warn(msg string) {
	defaultLogger.Warn(msg)
}

// Warnf logs a formatted warning message using the default logger
func Warnf(format string, args ...interface{}) {
	defaultLogger.Warnf(format, args...)
}

// Error logs an error message using the default logger
func Error(msg string) {
	defaultLogger.Error(msg)
}

// Errorf logs a formatted error message using the default logger
func Errorf(format string, args ...interface{}) {
	defaultLogger.Errorf(format, args...)
}

// WithField adds a field to the default logger context
func WithField(key string, value interface{}) *Logger {
	return defaultLogger.WithField(key, value)
}

// WithFields adds multiple fields to the default logger context
func WithFields(fields map[string]interface{}) *Logger {
	return defaultLogger.WithFields(fields)
}

// LoggerAdapter adapts our Logger to stdlib log.Logger
type LoggerAdapter struct {
	logger *Logger
}

// NewLoggerAdapter creates a new logger adapter
func NewLoggerAdapter(logger *Logger) *LoggerAdapter {
	return &LoggerAdapter{logger: logger}
}

// Write implements io.Writer interface
func (a *LoggerAdapter) Write(p []byte) (n int, err error) {
	a.logger.Info(string(p))
	return len(p), nil
}

// StdLogger returns a standard library logger
func (l *Logger) StdLogger() *log.Logger {
	return log.New(NewLoggerAdapter(l), "", 0)
}
