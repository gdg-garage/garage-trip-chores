package logger

import (
	"log/slog"
	"os"
	"strings"
)

func New(conf Config) *slog.Logger {
	var level slog.Leveler
	switch strings.ToLower(conf.Level) {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{AddSource: conf.IncludeFile, Level: level}))
	logger.Debug("Logger initialized.", "level", level, "includeFile", conf.IncludeFile)
	return logger
}
