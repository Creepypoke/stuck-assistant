package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCLIConfigPrepareValidateAndErrors(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "guide.txt"), []byte("# Начало\n\nКод: 0392."), 0600)
	cfg := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfg, []byte("game_id: test-game\ngame_name: Test\ninput: guide.txt\nartifacts: artifacts\noutput: test-game.rag\n"), 0600)
	cliCfg := filepath.Join(dir, "cli.yaml")
	os.WriteFile(cliCfg, []byte("providers: {}\n"), 0600)
	for _, action := range []string{"prepare", "validate"} {
		c := command()
		c.SetOut(&bytes.Buffer{})
		c.SetArgs([]string{"--config", cliCfg, "--walkthrough", cfg, action})
		if e := c.Execute(); e != nil {
			t.Fatal(e)
		}
	}
	for _, args := range [][]string{{"--config", cliCfg, "build"}, {"--config", cliCfg, "prepare"}, {"--config", cliCfg, "validate", "unexpected"}, {"--config", filepath.Join(dir, "missing"), "validate"}} {
		c := command()
		c.SetArgs(append([]string{"--walkthrough", cfg}, args...))
		if c.Execute() == nil {
			t.Fatalf("expected error %v", args)
		}
	}
}

func TestWalkthroughFlagRequired(t *testing.T) {
	c := command()
	c.SetArgs([]string{"prepare"})
	if e := c.Execute(); e == nil {
		t.Fatal("missing walkthrough config accepted")
	}
}
