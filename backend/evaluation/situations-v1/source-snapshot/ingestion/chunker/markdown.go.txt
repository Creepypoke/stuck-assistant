package chunker

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/walkthrough-assistance/backend/ingestion"
)

const Version = "markdown-v1"

var headingPattern = regexp.MustCompile(`^(#{1,3})\s+(.+?)\s*$`)

type Config struct {
	MaxTokens     int
	OverlapTokens int
}

type Markdown struct {
	maxTokens     int
	overlapTokens int
}

type section struct {
	title      string
	paragraphs []string
}

func New(config Config) (*Markdown, error) {
	if config.MaxTokens <= 0 {
		return nil, errorsf("max tokens must be positive")
	}
	if config.OverlapTokens < 0 || config.OverlapTokens >= config.MaxTokens {
		return nil, errorsf("overlap tokens must be non-negative and smaller than max tokens")
	}
	return &Markdown{maxTokens: config.MaxTokens, overlapTokens: config.OverlapTokens}, nil
}

func (c *Markdown) Version() string { return Version }

func (c *Markdown) Chunk(document ingestion.ParsedDocument) ([]ingestion.Chunk, error) {
	markdown := strings.TrimSpace(strings.ReplaceAll(document.Markdown, "\r\n", "\n"))
	if markdown == "" {
		return nil, ingestion.ErrEmptyDocument
	}

	title := strings.TrimSpace(document.Title)
	sections := parseSections(markdown, &title)
	if title == "" {
		title = "Walkthrough"
	}

	var chunks []ingestion.Chunk
	for _, section := range sections {
		texts := c.chunkParagraphs(section.paragraphs)
		for _, text := range texts {
			chunks = append(chunks, ingestion.Chunk{
				Title:    title,
				Section:  section.title,
				Sequence: len(chunks),
				Text:     text,
				Metadata: map[string]any{"token_estimate": estimateTokens(text)},
			})
		}
	}
	if len(chunks) == 0 {
		return nil, ingestion.ErrNoChunks
	}
	return chunks, nil
}

func parseSections(markdown string, documentTitle *string) []section {
	var sections []section
	current := section{}
	var paragraph []string

	flushParagraph := func() {
		if len(paragraph) == 0 {
			return
		}
		current.paragraphs = append(current.paragraphs, strings.Join(paragraph, "\n"))
		paragraph = nil
	}
	flushSection := func() {
		flushParagraph()
		if len(current.paragraphs) > 0 {
			sections = append(sections, current)
		}
		current = section{}
	}

	for _, line := range strings.Split(markdown, "\n") {
		trimmed := strings.TrimSpace(line)
		if match := headingPattern.FindStringSubmatch(trimmed); match != nil {
			flushSection()
			heading := strings.TrimSpace(match[2])
			if len(match[1]) == 1 && *documentTitle == "" {
				*documentTitle = heading
			}
			current.title = heading
			continue
		}
		if trimmed == "" {
			flushParagraph()
			continue
		}
		paragraph = append(paragraph, strings.TrimRight(line, " \t"))
	}
	flushSection()
	return sections
}

func (c *Markdown) chunkParagraphs(paragraphs []string) []string {
	var result []string
	var current []string

	for _, paragraph := range paragraphs {
		if estimateTokens(paragraph) > c.maxTokens {
			if len(current) > 0 {
				result = append(result, strings.Join(current, "\n\n"))
			}
			for _, part := range splitText(paragraph, c.maxTokens, c.overlapTokens) {
				result = append(result, part)
			}
			current = nil
			continue
		}
		candidate := append(append([]string(nil), current...), paragraph)
		if len(current) > 0 && estimateTokens(strings.Join(candidate, "\n\n")) > c.maxTokens {
			previous := strings.Join(current, "\n\n")
			result = append(result, previous)
			current = overlapTail(previous, c.overlapTokens)
			withOverlap := append(append([]string(nil), current...), paragraph)
			if estimateTokens(strings.Join(withOverlap, "\n\n")) > c.maxTokens {
				current = nil
			}
		}
		current = append(current, paragraph)
	}
	if len(current) > 0 {
		result = append(result, strings.Join(current, "\n\n"))
	}
	return result
}

func splitText(text string, maxTokens, overlapTokens int) []string {
	words := strings.Fields(text)
	var result []string
	for start := 0; start < len(words); {
		end := start
		for end < len(words) {
			candidate := strings.Join(words[start:end+1], " ")
			if end > start && estimateTokens(candidate) > maxTokens {
				break
			}
			end++
		}
		result = append(result, strings.Join(words[start:end], " "))
		if end == len(words) {
			break
		}
		next := end
		for next > start && estimateTokens(strings.Join(words[next-1:end], " ")) < overlapTokens {
			next--
		}
		if next == start {
			next = end
		}
		start = next
	}
	return result
}

func overlapTail(text string, tokens int) []string {
	if tokens == 0 {
		return nil
	}
	words := strings.Fields(text)
	start := len(words)
	for start > 0 && estimateTokens(strings.Join(words[start-1:], " ")) <= tokens {
		start--
	}
	if start == len(words) {
		return nil
	}
	return []string{strings.Join(words[start:], " ")}
}

// estimateTokens is intentionally tokenizer-independent and deterministic.
func estimateTokens(text string) int {
	runes := utf8.RuneCountInString(text)
	return (runes + 3) / 4
}

func errorsf(message string) error { return fmt.Errorf("invalid chunker config: %s", message) }
