package logger

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
)

type contextKey string

const loggerKey contextKey = "logger"

var globalLogger zerolog.Logger

// Initialize sets up the global logger
func Initialize(level string, format string) {
	// Configure output
	var output io.Writer = os.Stdout
	if format == "console" {
		output = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		}
	}

	// Parse level
	logLevel := zerolog.InfoLevel
	switch level {
	case "debug":
		logLevel = zerolog.DebugLevel
	case "info":
		logLevel = zerolog.InfoLevel
	case "warn":
		logLevel = zerolog.WarnLevel
	case "error":
		logLevel = zerolog.ErrorLevel
	}

	zerolog.SetGlobalLevel(logLevel)
	globalLogger = zerolog.New(output).With().Timestamp().Logger()
}

// Get returns the global logger
func Get() *zerolog.Logger {
	return &globalLogger
}

// FromContext retrieves logger from context
func FromContext(ctx context.Context) *zerolog.Logger {
	if logger, ok := ctx.Value(loggerKey).(*zerolog.Logger); ok {
		return logger
	}
	return &globalLogger
}

// WithContext adds logger to context
func WithContext(ctx context.Context, logger *zerolog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

// WithRequestID creates a logger with request ID
func WithRequestID(ctx context.Context, requestID string) context.Context {
	logger := FromContext(ctx).With().Str("request_id", requestID).Logger()
	return WithContext(ctx, &logger)
}
