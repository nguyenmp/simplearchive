package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/nguyenmp/simplearchive/internal/cli"
	"github.com/nguyenmp/simplearchive/internal/logging"
	"github.com/nguyenmp/simplearchive/internal/meta"
)

func main() {
	level := os.Getenv("LOG_LEVEL")
	logger := logging.New(level)
	slog.SetDefault(logger)

	logger.Info("starting simplearchive", "log_level", level)

	const dbPath = "./meta.db"
	db, err := meta.Open(context.Background(), dbPath)
	if err != nil {
		logger.Error("failed to open meta.db", "path", dbPath, "err", err)
		os.Exit(1)
	}
	defer db.Close()
	logger.Info("opened meta.db", "path", dbPath)

	cliApp := &cli.CLI{Logger: logger, DB: db}
	os.Exit(cliApp.Run(context.Background(), os.Args[1:]))
}
