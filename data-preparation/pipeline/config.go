package pipeline

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type Provider struct {
	Type            string `yaml:"type"`
	BaseURL         string `yaml:"base_url"`
	Model           string `yaml:"model"`
	APIKeyEnv       string `yaml:"api_key_env"`
	Timeout         string `yaml:"timeout"`
	Dimensions      int    `yaml:"dimensions,omitempty"`
	Version         string `yaml:"version,omitempty"`
	MaxOutputTokens int    `yaml:"max_output_tokens,omitempty"`
}
type Config struct {
	GameID             string              `yaml:"game_id"`
	GameName           string              `yaml:"game_name"`
	Input              string              `yaml:"input"`
	Artifacts          string              `yaml:"artifacts"`
	Output             string              `yaml:"output"`
	AnnotationProvider string              `yaml:"annotation_provider"`
	EmbeddingProvider  string              `yaml:"embedding_provider"`
	Providers          map[string]Provider `yaml:"providers"`
	MaxInputBytes      int64               `yaml:"max_input_bytes"`
	MaxBatchChars      int                 `yaml:"max_batch_chars"`
	RequireReview      bool                `yaml:"require_review"`
}

func decode(data []byte, out any) error {
	d := yaml.NewDecoder(bytes.NewReader(data))
	d.KnownFields(true)
	if err := d.Decode(out); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return fmt.Errorf("expected one YAML document")
	}
	return nil
}

// CLIConfig is shared by all walkthroughs: API connections and model profiles.
type CLIConfig struct {
	Providers map[string]Provider `yaml:"providers"`
}

// WalkthroughConfig contains only the input, output and processing policy for one guide.
type WalkthroughConfig struct {
	GameID             string `yaml:"game_id"`
	GameName           string `yaml:"game_name"`
	Input              string `yaml:"input"`
	Artifacts          string `yaml:"artifacts"`
	Output             string `yaml:"output"`
	AnnotationProvider string `yaml:"annotation_provider"`
	EmbeddingProvider  string `yaml:"embedding_provider"`
	MaxInputBytes      int64  `yaml:"max_input_bytes"`
	MaxBatchChars      int    `yaml:"max_batch_chars"`
	RequireReview      bool   `yaml:"require_review"`
}

func readConfig(path string, out any) error {
	b, e := os.ReadFile(path)
	if e == nil {
		e = decode(b, out)
	}
	if e != nil {
		return fmt.Errorf("config %s: %w", path, e)
	}
	return nil
}

// LoadConfig resolves paths against the walkthrough file, never the CLI file or cwd.
func LoadConfig(cliPath, walkthroughPath string) (Config, error) {
	var cli CLIConfig
	w := WalkthroughConfig{MaxInputBytes: 32 << 20, MaxBatchChars: 12000, RequireReview: true}
	if e := readConfig(cliPath, &cli); e != nil {
		return Config{}, e
	}
	if e := readConfig(walkthroughPath, &w); e != nil {
		return Config{}, e
	}
	c := Config{GameID: w.GameID, GameName: w.GameName, Input: w.Input, Artifacts: w.Artifacts, Output: w.Output, AnnotationProvider: w.AnnotationProvider, EmbeddingProvider: w.EmbeddingProvider, Providers: cli.Providers, MaxInputBytes: w.MaxInputBytes, MaxBatchChars: w.MaxBatchChars, RequireReview: w.RequireReview}
	if !slug.MatchString(c.GameID) || c.GameName == "" || c.Input == "" || c.Artifacts == "" || c.Output == "" {
		return c, fmt.Errorf("walkthrough config: game_id (slug), game_name, input, artifacts and output are required")
	}
	if c.MaxInputBytes <= 0 || c.MaxBatchChars < 256 {
		return c, fmt.Errorf("invalid input/batch limits")
	}
	base, e := filepath.Abs(filepath.Dir(walkthroughPath))
	if e != nil {
		return c, e
	}
	for _, p := range []*string{&c.Input, &c.Artifacts, &c.Output} {
		if !filepath.IsAbs(*p) {
			*p = filepath.Join(base, *p)
		}
	}
	for name, p := range c.Providers {
		if p.Type != "openai" && p.Type != "anthropic" {
			return c, fmt.Errorf("provider %s: type must be openai or anthropic", name)
		}
		if p.Model == "" || p.BaseURL == "" {
			return c, fmt.Errorf("provider %s: model and base_url required", name)
		}
		if p.Timeout == "" {
			p.Timeout = "2m"
		}
		d, e := time.ParseDuration(p.Timeout)
		if e != nil || d <= 0 {
			return c, fmt.Errorf("provider %s: invalid timeout", name)
		}
		if p.MaxOutputTokens == 0 {
			p.MaxOutputTokens = 4096
		}
		c.Providers[name] = p
	}
	for _, name := range []string{c.AnnotationProvider, c.EmbeddingProvider} {
		if name != "" {
			if _, ok := c.Providers[name]; !ok {
				return c, fmt.Errorf("walkthrough references unknown provider %q", name)
			}
		}
	}
	return c, nil
}

func WriteYAML(path string, v any) error {
	b, e := yaml.Marshal(v)
	if e != nil {
		return e
	}
	return os.WriteFile(path, b, 0644)
}
