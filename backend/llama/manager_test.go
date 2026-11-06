package llama

import "testing"

func TestNewManagerValidatesConfiguration(t *testing.T) {
	if _, err := NewManager(Config{HealthURL: "http://127.0.0.1/health"}); err == nil {
		t.Fatal("expected missing command error")
	}
	if _, err := NewManager(Config{Command: "llama"}); err == nil {
		t.Fatal("expected missing health URL error")
	}
}
