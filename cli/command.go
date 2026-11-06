package cli

import (
	"fmt"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func NewCommand() *cobra.Command {
	logLevel := environmentOrDefault("WALKTHROUGH_LOG_LEVEL", "info")
	root := &cobra.Command{Use: "walkthrough-assistance", Short: "Search prepared walkthrough knowledge and manage the service", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().StringVar(&logLevel, "log-level", logLevel, "logging level: debug, info, warn, or error")
	root.AddCommand(newServiceCommand(), newAskCommand())
	return root
}
func resolveExecutableRelativePath(path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	executable, err := os.Executable()
	if err != nil {
		return path
	}
	return filepath.Join(filepath.Dir(executable), path)
}

func resolveExecutableRelativePaths(paths []string) []string {
	for index, path := range paths {
		paths[index] = resolveExecutableRelativeCommand(path)
	}
	return paths
}

// resolveExecutableRelativeCommand keeps command names and option values intact.
// A relative file path must contain a directory component (for example, ./parser.py).
func resolveExecutableRelativeCommand(value string) string {
	if value == "" || strings.HasPrefix(value, "-") || !strings.ContainsRune(value, filepath.Separator) {
		return value
	}
	return resolveExecutableRelativePath(value)
}

func environmentOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func environmentInt(key string, defaultValue int) (int, error) {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed, nil
}

func environmentDuration(key string, defaultValue time.Duration) (time.Duration, error) {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed, nil
}

func defaultStorageDirectory() string {
	dataDirectory, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("data", "games")
	}
	return filepath.Join(dataDirectory, ".local", "share", "walkthrough-assistance", "games")
}
