package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const serviceUnitName = "walkthrough-assistance.service"

type commandRunner func(context.Context, string, ...string) ([]byte, error)

func newServiceCommand() *cobra.Command {
	return newServiceCommandWithRunner(runCommand)
}

func newServiceCommandWithRunner(runner commandRunner) *cobra.Command {
	command := &cobra.Command{Use: "service", Short: "Install and manage the background service"}
	command.AddCommand(newServiceInstallCommand(runner))
	for _, action := range []string{"start", "stop", "restart", "enable", "disable"} {
		action := action
		command.AddCommand(&cobra.Command{
			Use: action, Short: action + " the background service", Args: cobra.NoArgs,
			RunE: func(command *cobra.Command, _ []string) error {
				arguments := []string{"--user", action, serviceUnitName}
				if action == "enable" {
					arguments = []string{"--user", "enable", "--now", serviceUnitName}
				}
				output, err := runner(command.Context(), "systemctl", arguments...)
				if len(output) > 0 {
					_, _ = command.OutOrStdout().Write(output)
				}
				if err != nil {
					return fmt.Errorf("systemctl %s: %w", action, err)
				}
				return nil
			},
		})
	}
	command.AddCommand(&cobra.Command{
		Use: "status", Short: "Show whether the background service is enabled and running", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			enabled, enabledErr := runner(command.Context(), "systemctl", "--user", "is-enabled", serviceUnitName)
			active, activeErr := runner(command.Context(), "systemctl", "--user", "is-active", serviceUnitName)
			fmt.Fprintf(command.OutOrStdout(), "enabled: %s\nactive: %s\n", cleanStatus(enabled, enabledErr), cleanStatus(active, activeErr))
			return nil
		},
	})
	return command
}

func newServiceInstallCommand(runner commandRunner) *cobra.Command {
	var serviceBinary string
	var environmentFile string
	command := &cobra.Command{
		Use: "install", Short: "Install and start the systemd user service", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if serviceBinary == "" {
				path, err := exec.LookPath("walkthrough-assistance-service")
				if err != nil {
					return errors.New("walkthrough-assistance-service not found; pass --service-binary")
				}
				serviceBinary = path
			}
			absoluteBinary, err := filepath.Abs(serviceBinary)
			if err != nil {
				return fmt.Errorf("resolve service binary: %w", err)
			}
			if environmentFile != "" {
				environmentFile, err = filepath.Abs(environmentFile)
				if err != nil {
					return fmt.Errorf("resolve environment file: %w", err)
				}
			}
			unitDirectory, err := userUnitDirectory()
			if err != nil {
				return err
			}
			if err := os.MkdirAll(unitDirectory, 0o755); err != nil {
				return fmt.Errorf("create systemd user directory: %w", err)
			}
			unitPath := filepath.Join(unitDirectory, serviceUnitName)
			if err := os.WriteFile(unitPath, []byte(serviceUnit(absoluteBinary, environmentFile)), 0o644); err != nil {
				return fmt.Errorf("write systemd unit: %w", err)
			}
			if _, err := runner(command.Context(), "systemctl", "--user", "daemon-reload"); err != nil {
				return fmt.Errorf("reload systemd user units: %w", err)
			}
			if _, err := runner(command.Context(), "systemctl", "--user", "enable", "--now", serviceUnitName); err != nil {
				return fmt.Errorf("enable service: %w", err)
			}
			fmt.Fprintf(command.OutOrStdout(), "installed: %s\n", unitPath)
			return nil
		},
	}
	command.Flags().StringVar(&serviceBinary, "service-binary", "", "path to walkthrough-assistance-service")
	command.Flags().StringVar(&environmentFile, "environment-file", "", "optional service environment file")
	return command
}

func runCommand(ctx context.Context, name string, arguments ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, arguments...)
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	err := command.Run()
	return output.Bytes(), err
}

func userUnitDirectory() (string, error) {
	if configDirectory := os.Getenv("XDG_CONFIG_HOME"); configDirectory != "" {
		return filepath.Join(configDirectory, "systemd", "user"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".config", "systemd", "user"), nil
}

func serviceUnit(binary, environmentFile string) string {
	environment := ""
	if environmentFile != "" {
		environment = "EnvironmentFile=-" + systemdEscape(environmentFile) + "\n"
	}
	return "[Unit]\n" +
		"Description=Walkthrough Assistance service\n" +
		"After=network.target\n\n" +
		"[Service]\n" + environment +
		"ExecStart=" + systemdEscape(binary) + "\n" +
		"Restart=on-failure\n" +
		"RestartSec=3\n\n" +
		"[Install]\n" +
		"WantedBy=default.target\n"
}

func systemdEscape(value string) string {
	value = strings.ReplaceAll(value, "%", "%%")
	value = strings.ReplaceAll(value, "\\", "\\x5c")
	return strings.ReplaceAll(value, " ", "\\x20")
}

func cleanStatus(output []byte, err error) string {
	status := strings.TrimSpace(string(output))
	if status != "" {
		return status
	}
	if err != nil {
		return "unknown"
	}
	return "unknown"
}
