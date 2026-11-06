package llamacpp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type EmbeddingInfo struct {
	Model      string
	Version    string
	Dimensions int
}

type Provider struct {
	endpoint string
	client   *http.Client
	info     EmbeddingInfo
}

type Config struct {
	BaseURL    string
	Model      string
	Version    string
	Dimensions int
	Client     *http.Client
	Timeout    time.Duration
}

func New(config Config) (*Provider, error) {
	baseURL, err := url.Parse(strings.TrimRight(config.BaseURL, "/"))
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" {
		return nil, fmt.Errorf("invalid llama.cpp base URL %q", config.BaseURL)
	}
	if config.Model == "" || config.Version == "" || config.Dimensions <= 0 {
		return nil, fmt.Errorf("embedding model, version, and positive dimensions are required")
	}
	client := config.Client
	if client == nil {
		timeout := config.Timeout
		if timeout <= 0 {
			timeout = 2 * time.Minute
		}
		client = &http.Client{Timeout: timeout}
	}
	return &Provider{
		endpoint: strings.TrimRight(baseURL.String(), "/") + "/v1/embeddings",
		client:   client,
		info: EmbeddingInfo{
			Model: config.Model, Dimensions: config.Dimensions, Version: config.Version,
		},
	}, nil
}

func (p *Provider) Info() EmbeddingInfo { return p.info }

func (p *Provider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	payload, err := json.Marshal(struct {
		Model string   `json:"model"`
		Input []string `json:"input"`
	}{Model: p.info.Model, Input: texts})
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := p.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("embedding request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("embedding request: unexpected HTTP status %s", response.Status)
	}
	var body struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode embedding response: %w", err)
	}
	if len(body.Data) != len(texts) {
		return nil, fmt.Errorf("embedding response contains %d vectors for %d texts", len(body.Data), len(texts))
	}
	vectors := make([][]float32, len(texts))
	seen := make([]bool, len(texts))
	for _, item := range body.Data {
		if item.Index < 0 || item.Index >= len(vectors) || seen[item.Index] || len(item.Embedding) != p.info.Dimensions {
			return nil, fmt.Errorf("embedding response contains invalid vector at index %d", item.Index)
		}
		vectors[item.Index] = item.Embedding
		seen[item.Index] = true
	}
	return vectors, nil
}
