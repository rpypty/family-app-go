package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
)

const (
	LevelCritical = slog.Level(12)
)

type Logger interface {
	Debug(message string, args ...any)
	Info(message string, args ...any)
	Warn(message string, args ...any)
	Error(message string, args ...any)
	Critical(message string, args ...any)
	BusinessError(message string, err error, args ...any)
	InternalError(message string, err error, args ...any)
	With(args ...any) Logger
}

type slogLogger struct {
	base *slog.Logger
}

func NewFromEnv() Logger {
	env := normalizeValue(os.Getenv("ENV"))
	level := parseLevel(os.Getenv("LOG_LEVEL"), env)
	format := parseFormat(os.Getenv("LOG_FORMAT"))
	return New(os.Stdout, level, format)
}

func New(output io.Writer, level slog.Level, format string) Logger {
	options := &slog.HandlerOptions{
		Level:       level,
		ReplaceAttr: replaceAttr,
	}

	var handler slog.Handler
	switch normalizeValue(format) {
	case "json":
		handler = slog.NewJSONHandler(output, options)
	default:
		handler = slog.NewTextHandler(output, options)
	}

	return &slogLogger{base: slog.New(handler)}
}

func (l *slogLogger) Debug(message string, args ...any) {
	l.base.Debug(message, args...)
}

func (l *slogLogger) Info(message string, args ...any) {
	l.base.Info(message, args...)
}

func (l *slogLogger) Warn(message string, args ...any) {
	l.base.Warn(message, args...)
}

func (l *slogLogger) Error(message string, args ...any) {
	l.base.Error(message, args...)
}

func (l *slogLogger) Critical(message string, args ...any) {
	l.base.Log(context.Background(), LevelCritical, message, args...)
}

func (l *slogLogger) BusinessError(message string, err error, args ...any) {
	if err == nil {
		return
	}

	attrs := append([]any{"err", err}, args...)
	l.base.Warn(message, attrs...)
}

func (l *slogLogger) InternalError(message string, err error, args ...any) {
	if err == nil {
		return
	}

	attrs := append([]any{"err", err}, args...)
	l.base.Error(message, attrs...)
}

func (l *slogLogger) With(args ...any) Logger {
	return &slogLogger{base: l.base.With(args...)}
}

func parseLevel(value string, env string) slog.Level {
	switch normalizeValue(value) {
	case "debug":
		return slog.LevelDebug
	case "info", "":
		if env == "development" {
			return slog.LevelDebug
		}
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	case "critical", "fatal":
		return LevelCritical
	default:
		if env == "development" {
			return slog.LevelDebug
		}
		return slog.LevelInfo
	}
}

func parseFormat(value string) string {
	switch normalizeValue(value) {
	case "json", "text":
		return normalizeValue(value)
	default:
		return "json"
	}
}

func normalizeValue(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func replaceAttr(_ []string, attr slog.Attr) slog.Attr {
	if attr.Key != slog.LevelKey {
		return attr
	}

	level, ok := attr.Value.Any().(slog.Level)
	if !ok {
		return attr
	}

	if level == LevelCritical {
		attr.Value = slog.StringValue("CRITICAL")
	}
	return attr
}
