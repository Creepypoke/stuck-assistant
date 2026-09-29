package llamacpp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEmbedBatchRestoresResponseIndexOrder(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"data":[{"index":1,"embedding":[3,4]},{"index":0,"embedding":[1,2]}]}`))
	}))
	defer server.Close()
	provider, err := New(Config{BaseURL: server.URL, Model: "test", Version: "1", Dimensions: 2})
	if err != nil {
		t.Fatal(err)
	}
	vectors, err := provider.EmbedBatch(context.Background(), []string{"one", "two"})
	if err != nil {
		t.Fatal(err)
	}
	if vectors[0][0] != 1 || vectors[1][0] != 3 {
		t.Fatalf("vectors are in wrong order: %#v", vectors)
	}
}
