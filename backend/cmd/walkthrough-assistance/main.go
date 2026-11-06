package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/walkthrough-assistance/backend/cli"
	"github.com/walkthrough-assistance/backend/logging"
)

func main() {
	executablePath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve executable path: %v\n", err)
		os.Exit(1)
	}
	if err := godotenv.Load(filepath.Join(filepath.Dir(executablePath), ".env")); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "load .env: %v\n", err)
		os.Exit(1)
	}
	logger, err := logging.New(os.Stderr, os.Getenv("WALKTHROUGH_LOG_LEVEL"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := cli.NewCommand().ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
