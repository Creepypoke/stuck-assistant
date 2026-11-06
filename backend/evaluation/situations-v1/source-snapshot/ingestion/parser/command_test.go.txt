package parser

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/walkthrough-assistance/backend/ingestion"
)

func TestCommandParser(t *testing.T) {
	t.Parallel()
	parser := NewCommand(os.Args[0], "-test.run=TestParserHelper", "--", "parser-helper")
	result, err := parser.Parse(context.Background(), ingestion.RawDocument{Content: []byte("<h1>Guide</h1>")})
	if err != nil {
		t.Fatal(err)
	}
	if result.Title != "Guide" || result.Markdown != "# Guide\n\nBody" || result.Metadata["language"] != "en" {
		t.Fatalf("unexpected parser result: %#v", result)
	}
}

func TestCommandParserUsesPlainTextAsMarkdown(t *testing.T) {
	t.Parallel()
	parser := NewCommand("")
	parsed, err := parser.Parse(context.Background(), ingestion.RawDocument{
		ContentType: "text/plain; charset=utf-8",
		Content:     []byte("Open the gate.\r\nThen enter the castle.\r\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Markdown != "Open the gate.\nThen enter the castle." {
		t.Fatalf("unexpected markdown: %q", parsed.Markdown)
	}
}

func TestParserHelper(t *testing.T) {
	if len(os.Args) < 2 || os.Args[len(os.Args)-1] != "parser-helper" {
		return
	}
	fmt.Print(`{"title":" Guide ","markdown":"# Guide\r\n\r\nBody\r\n","metadata":{"language":"en"}}`)
	os.Exit(0)
}
