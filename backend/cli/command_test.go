package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveExecutableRelativePath(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(filepath.Dir(executable), "data", "games")
	if actual := resolveExecutableRelativePath(filepath.Join("data", "games")); actual != expected {
		t.Fatalf("relative path = %q, want %q", actual, expected)
	}
	if actual := resolveExecutableRelativePath("/tmp/games"); actual != "/tmp/games" {
		t.Fatalf("absolute path = %q", actual)
	}
}

func TestResolveExecutableRelativeCommand(t *testing.T) {
	if actual := resolveExecutableRelativeCommand("python"); actual != "python" {
		t.Fatalf("command name = %q, want python", actual)
	}
	if actual := resolveExecutableRelativeCommand("--format"); actual != "--format" {
		t.Fatalf("option = %q, want --format", actual)
	}
	paths := resolveExecutableRelativePaths([]string{"./parser.py", "--format", "markdown"})
	if paths[1] != "--format" || paths[2] != "markdown" || !filepath.IsAbs(paths[0]) {
		t.Fatalf("unexpected resolved arguments: %#v", paths)
	}
}

func TestPreparationCommandRemoved(t *testing.T) {
	c := NewCommand()
	c.SetArgs([]string{"knowledge", "add"})
	if c.Execute() == nil {
		t.Fatal("backend must not prepare data")
	}
}
