package chunker

import (
	"strings"
	"testing"

	"github.com/walkthrough-assistance/backend/ingestion"
)

func TestChunkPreservesHeadingBoundariesAndMetadata(t *testing.T) {
	t.Parallel()
	chunker, err := New(Config{MaxTokens: 50, OverlapTokens: 5})
	if err != nil {
		t.Fatal(err)
	}
	chunks, err := chunker.Chunk(ingestion.ParsedDocument{Markdown: `# Main Quest

Introductory paragraph.

## Entering the Castle

Find the gate and talk to the guard.

## Finding the Key

Search the table in the northern room.`})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 3 {
		t.Fatalf("got %d chunks, want 3: %#v", len(chunks), chunks)
	}
	if chunks[0].Title != "Main Quest" || chunks[1].Section != "Entering the Castle" || chunks[2].Sequence != 2 {
		t.Fatalf("heading metadata was not preserved: %#v", chunks)
	}
}

func TestChunkSplitsOversizedSectionWithinLimit(t *testing.T) {
	t.Parallel()
	chunker, err := New(Config{MaxTokens: 12, OverlapTokens: 2})
	if err != nil {
		t.Fatal(err)
	}
	chunks, err := chunker.Chunk(ingestion.ParsedDocument{
		Title: "Guide", Markdown: "## Route\n\n" + strings.Repeat("word ", 40),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	for _, chunk := range chunks {
		if tokens := estimateTokens(chunk.Text); tokens > 12 {
			t.Errorf("chunk %d has %d estimated tokens", chunk.Sequence, tokens)
		}
	}
}
