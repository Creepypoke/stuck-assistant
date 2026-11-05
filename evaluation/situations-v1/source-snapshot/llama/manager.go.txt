package llama

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"
)

type Config struct {
	Command       string
	Args          []string
	HealthURL     string
	HealthTimeout time.Duration
}

type Status struct {
	Running bool   `json:"running"`
	Healthy bool   `json:"healthy"`
	Error   string `json:"error,omitempty"`
}

type Manager struct {
	config Config
	client *http.Client

	mu      sync.RWMutex
	command *exec.Cmd
	running bool
	waitErr error
	done    chan error
}

func NewManager(config Config) (*Manager, error) {
	if config.Command == "" {
		return nil, errors.New("llama command is required")
	}
	if config.HealthURL == "" {
		return nil, errors.New("llama health URL is required")
	}
	if config.HealthTimeout <= 0 {
		config.HealthTimeout = 2 * time.Second
	}
	return &Manager{
		config: config,
		client: &http.Client{Timeout: config.HealthTimeout},
		done:   make(chan error, 1),
	}, nil
}

func (manager *Manager) Start(ctx context.Context) error {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.running {
		return errors.New("llama process is already running")
	}

	command := exec.CommandContext(ctx, manager.config.Command, manager.config.Args...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		return fmt.Errorf("start llama.cpp: %w", err)
	}
	manager.command = command
	manager.running = true
	manager.waitErr = nil
	go manager.wait(command)
	return nil
}

func (manager *Manager) wait(command *exec.Cmd) {
	err := command.Wait()
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.command == command {
		manager.running = false
		manager.waitErr = err
	}
	manager.done <- err
}

func (manager *Manager) Done() <-chan error {
	return manager.done
}

func (manager *Manager) Status(ctx context.Context) Status {
	manager.mu.RLock()
	waitErr := manager.waitErr
	running := manager.running
	manager.mu.RUnlock()

	status := Status{Running: running}
	if !running {
		if waitErr != nil {
			status.Error = waitErr.Error()
		}
		return status
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, manager.config.HealthURL, nil)
	if err != nil {
		status.Error = err.Error()
		return status
	}
	response, err := manager.client.Do(request)
	if err != nil {
		status.Error = err.Error()
		return status
	}
	defer response.Body.Close()
	status.Healthy = response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices
	if !status.Healthy {
		status.Error = response.Status
	}
	return status
}
