package main

import (
	"log/slog"
	"os"

	"github.com/nguyenmp/simplearchive/internal/logging"
)

func main() {
	level := os.Getenv("LOG_LEVEL")
	logger := logging.New(level)
	slog.SetDefault(logger)

	logger.Info("starting simplearchive", "log_level", level)
}