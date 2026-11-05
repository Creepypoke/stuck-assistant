package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServiceInstallWritesAndEnablesUserUnit(t *testing.T) {
	configDirectory := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDirectory)
	serviceBinary := filepath.Join(t.TempDir(), "walkthrough-assistance-service")
	if err := os.WriteFile(serviceBinary, nil, 0o755); err != nil {
		t.Fatal(err)
	}

	var calls []string
	runner := func(_ context.Context, name string, arguments ...string) ([]byte, error) {
		calls = append(calls, name+" "+strings.Join(arguments, " "))
		return nil, nil
	}
	command := newServiceCommandWithRunner(runner)
	output := new(bytes.Buffer)
	command.SetOut(output)
	command.SetArgs([]string{"install", "--service-binary", serviceBinary})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	unitPath := filepath.Join(configDirectory, "systemd", "user", serviceUnitName)
	contents, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "ExecStart="+serviceBinary) || !strings.Contains(string(contents), "WantedBy=default.target") {
		t.Fatalf("unexpected unit:\n%s", contents)
	}
	if len(calls) != 2 || calls[0] != "systemctl --user daemon-reload" ||
		calls[1] != "systemctl --user enable --now "+serviceUnitName {
		t.Fatalf("unexpected systemctl calls: %#v", calls)
	}
}

func TestServiceStatusReportsSystemdState(t *testing.T) {
	runner := func(_ context.Context, _ string, arguments ...string) ([]byte, error) {
		if arguments[1] == "is-enabled" {
			return []byte("enabled\n"), nil
		}
		return []byte("active\n"), nil
	}
	command := newServiceCommandWithRunner(runner)
	output := new(bytes.Buffer)
	command.SetOut(output)
	command.SetArgs([]string{"status"})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if output.String() != "enabled: enabled\nactive: active\n" {
		t.Fatalf("unexpected status: %q", output.String())
	}
}
