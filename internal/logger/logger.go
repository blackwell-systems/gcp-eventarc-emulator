// Package logger provides a simple leveled logger for the eventarc emulator.
// It wraps the standard log package and adds level filtering so that
// --log-level debug produces debug output while the default info level
// suppresses it.
package logger

import (
	"log"
	"strings"
)

// level represents the logging verbosity level.
type level int

const (
	levelDebug level = iota
	levelInfo
	levelWarn
	levelError
)

// Logger is a leveled logger that filters messages below its configured level.
type Logger struct {
	lvl level
}

// New creates a Logger for the given level string.
// Accepted values (case-insensitive): "debug", "info", "warn", "warning", "error".
// Any unrecognised value defaults to info.
func New(lvlStr string) *Logger {
	var l level
	switch strings.ToLower(lvlStr) {
	case "debug":
		l = levelDebug
	case "warn", "warning":
		l = levelWarn
	case "error":
		l = levelError
	default:
		l = levelInfo
	}
	return &Logger{lvl: l}
}

// IsDebug reports whether the logger is configured at debug level.
func (l *Logger) IsDebug() bool {
	return l.lvl <= levelDebug
}

// Debug logs a message at debug level.
func (l *Logger) Debug(format string, args ...any) {
	if l.lvl <= levelDebug {
		log.Printf("[DEBUG] "+format, args...)
	}
}

// Info logs a message at info level.
func (l *Logger) Info(format string, args ...any) {
	if l.lvl <= levelInfo {
		log.Printf("[INFO] "+format, args...)
	}
}

// Warn logs a message at warn level.
func (l *Logger) Warn(format string, args ...any) {
	if l.lvl <= levelWarn {
		log.Printf("[WARN] "+format, args...)
	}
}

// Error logs a message at error level.
func (l *Logger) Error(format string, args ...any) {
	if l.lvl <= levelError {
		log.Printf("[ERROR] "+format, args...)
	}
}
