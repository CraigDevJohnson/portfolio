// Package logging provides shared slog bootstrap and request-context helpers.
package logging

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultFormat is the fallback slog output format.
	DefaultFormat = "json"
	// LevelEnvVar configures the default slog level.
	LevelEnvVar = "LOG_LEVEL"
	// FormatEnvVar configures the slog handler format.
	FormatEnvVar = "LOG_FORMAT"
	// AddSourceEnvVar controls whether slog emits source metadata.
	AddSourceEnvVar = "LOG_ADD_SOURCE"
)

type contextKey string

const requestIDKey contextKey = "request_id"

// Config describes runtime slog configuration.
type Config struct {
	AddSource bool
	Format    string
	Level     slog.Level
}

// Component returns a child logger with a stable component attribute.
func Component(name string) *slog.Logger {
	return slog.Default().With(slog.String("component", name))
}

// LoadConfigFromEnv reads slog configuration from environment variables.
func LoadConfigFromEnv() (Config, []string) {
	cfg := Config{
		AddSource: envBool(AddSourceEnvVar),
		Format:    DefaultFormat,
		Level:     slog.LevelInfo,
	}

	var warnings []string

	if rawFormat := strings.TrimSpace(os.Getenv(FormatEnvVar)); rawFormat != "" {
		switch strings.ToLower(rawFormat) {
		case "json":
			cfg.Format = "json"
		case "text":
			cfg.Format = "text"
		default:
			warnings = append(warnings, "invalid LOG_FORMAT="+rawFormat+"; using json")
		}
	}

	if rawLevel := strings.TrimSpace(os.Getenv(LevelEnvVar)); rawLevel != "" {
		var level slog.Level
		if err := level.UnmarshalText([]byte(rawLevel)); err != nil {
			warnings = append(warnings, "invalid LOG_LEVEL="+rawLevel+"; using info")
		} else {
			cfg.Level = level
		}
	}

	return cfg, warnings
}

// NewLoggerFromEnv constructs a slog logger using environment-driven settings.
func NewLoggerFromEnv() (*slog.Logger, Config, []string) {
	cfg, warnings := LoadConfigFromEnv()
	options := &slog.HandlerOptions{
		AddSource: cfg.AddSource,
		Level:     cfg.Level,
	}

	var handler slog.Handler
	switch cfg.Format {
	case "text":
		handler = slog.NewTextHandler(os.Stdout, options)
	default:
		handler = slog.NewJSONHandler(os.Stdout, options)
	}

	return slog.New(handler), cfg, warnings
}

// NewRequestID returns a random request ID suitable for log correlation.
func NewRequestID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err == nil {
		return hex.EncodeToString(buf[:])
	}
	return strconv.FormatInt(time.Now().UnixNano(), 16)
}

// RequestIDFromContext returns the request ID stored in ctx, if any.
func RequestIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey).(string)
	return strings.TrimSpace(value)
}

// WithContext adds request-scoped fields from ctx to logger.
func WithContext(logger *slog.Logger, ctx context.Context) *slog.Logger {
	if logger == nil {
		logger = slog.Default()
	}
	if requestID := RequestIDFromContext(ctx); requestID != "" {
		return logger.With(slog.String("request_id", requestID))
	}
	return logger
}

// WithRequestID stores requestID in ctx for downstream logging.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, strings.TrimSpace(requestID))
}

func envBool(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
