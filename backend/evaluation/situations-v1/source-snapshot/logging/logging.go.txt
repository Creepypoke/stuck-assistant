package logging

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// New creates a text logger that writes messages at level or above to output.
func New(output io.Writer, level string) (*slog.Logger, error) {
	var parsed slog.Level
	if level == "" {
		level = "info"
	}
	if err := parsed.UnmarshalText([]byte(strings.ToUpper(level))); err != nil {
		return nil, fmt.Errorf("parse log level %q: %w", level, err)
	}
	return slog.New(slog.NewTextHandler(output, &slog.HandlerOptions{Level: parsed})), nil
}
