package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

func yamlBytes(v any) ([]byte, error) { return yaml.Marshal(v) }
func Prepare(ctx context.Context, c Config) error {
	if _, e := os.Stat(c.Artifacts); !os.IsNotExist(e) {
		return fmt.Errorf("artifacts directory already exists or is inaccessible: %s", c.Artifacts)
	}
	raw, doc, blocks, warnings, e := Extract(c.Input, c.MaxInputBytes)
	if e != nil {
		return e
	}
	a := Artifacts{Raw: raw, Document: doc, Blocks: blocks, Manifest: Manifest{Version: Version, GameID: c.GameID, GameName: c.GameName, InputName: filepath.Base(c.Input), RawSHA256: hash(raw), DocumentSHA256: hash([]byte(doc)), AnnotationProvider: c.AnnotationProvider, MaxBatchChars: c.MaxBatchChars, Warnings: warnings}}
	if c.AnnotationProvider == "" {
		// Structural fallback: each exact source block is searchable, with its hierarchy.
		for _, b := range blocks {
			a.Cards = append(a.Cards, Card{ID: "s-" + b.ID, Task: b.Text, Episode: strings.Join(b.HeadingPath, " / "), BlockIDs: []string{b.ID}, ValidationStatus: "source_only"})
		}
	} else {
		p := c.Providers[c.AnnotationProvider]
		a.Manifest.AnnotationModel = p.Model
		a.Manifest.AnnotationProtocol = p.Type
		a.Manifest.AnnotationMaxOutputTokens = p.MaxOutputTokens
		a.Manifest.AnnotationPromptSHA256 = hash([]byte(annotationPrompt))
		for start := 0; start < len(blocks); {
			end := start
			size := 0
			for end < len(blocks) {
				n := utf8.RuneCountInString(blocks[end].Text)
				if n > c.MaxBatchChars {
					return fmt.Errorf("block %s exceeds max_batch_chars; split it at a meaningful boundary", blocks[end].ID)
				}
				if end > start && (size+n > c.MaxBatchChars || !reflect.DeepEqual(blocks[start].HeadingPath, blocks[end].HeadingPath)) {
					break
				}
				size += n
				end++
			}
			cards, e := annotate(ctx, p, blocks[start:end])
			if e != nil {
				return fmt.Errorf("annotate blocks %d..%d: %w", start, end, e)
			}
			// Restrict model output to this batch. Full document coverage is checked below.
			expected := []string{}
			actual := []string{}
			for _, b := range blocks[start:end] {
				expected = append(expected, b.ID)
			}
			for i := range cards {
				cards[i].ID = fmt.Sprintf("s-%05d", len(a.Cards)+i+1)
				cards[i].ValidationStatus = "needs_review"
				cards[i].SourceSpans = nil
				actual = append(actual, cards[i].BlockIDs...)
			}
			if !reflect.DeepEqual(expected, actual) {
				return fmt.Errorf("annotation must cover batch blocks exactly once in source order")
			}
			a.Cards = append(a.Cards, cards...)
			start = end
		}
	}
	if e = hydrate(&a); e != nil {
		return e
	}
	if e = validate(a, false); e != nil {
		return e
	}
	parent := filepath.Dir(c.Artifacts)
	if e = os.MkdirAll(parent, 0755); e != nil {
		return e
	}
	tmp, e := os.MkdirTemp(parent, ".prepare-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(tmp)
	for name, v := range map[string]any{"manifest.yaml": a.Manifest, "blocks.yaml": a.Blocks, "cards.yaml": a.Cards, "search.yaml": a.Entries} {
		if e = WriteYAML(filepath.Join(tmp, name), v); e != nil {
			return e
		}
	}
	if e = os.WriteFile(filepath.Join(tmp, "source.bin"), a.Raw, 0644); e != nil {
		return e
	}
	if e = os.WriteFile(filepath.Join(tmp, "document.txt"), []byte(doc), 0644); e != nil {
		return e
	}
	return os.Rename(tmp, c.Artifacts)
}
func hydrate(a *Artifacts) error {
	byID := map[string]Block{}
	for _, b := range a.Blocks {
		byID[b.ID] = b
	}
	a.Entries = nil
	for i := range a.Cards {
		card := &a.Cards[i]
		card.SourceSpans = nil
		quotes := []string{}
		for _, id := range card.BlockIDs {
			b, ok := byID[id]
			if !ok {
				return fmt.Errorf("card %s: unknown block %s", card.ID, id)
			}
			card.SourceSpans = append(card.SourceSpans, Span{a.Manifest.DocumentSHA256, b.ID, b.Start, b.End, b.Text})
			quotes = append(quotes, b.Text)
		}
		answer := strings.Join(quotes, "\n\n")
		answerSpans := card.SourceSpans
		// Reviewed facts may select a smaller set of exact source spans. Keep
		// the entire assigned source in search, and conditions in the answer.
		if card.ValidationStatus == "reviewed" && len(card.Facts) > 0 {
			answerSpans = nil
			seen := map[string]bool{}
			appendQuote := func(blockID, quote string) error {
				b, ok := byID[blockID]
				if !ok {
					return fmt.Errorf("unknown block %s", blockID)
				}
				pos := strings.Index(b.Text, quote)
				if quote == "" || pos < 0 {
					return fmt.Errorf("unsupported answer evidence")
				}
				start := b.Start + utf8.RuneCountInString(b.Text[:pos])
				key := fmt.Sprintf("%s:%d:%d", blockID, start, utf8.RuneCountInString(quote))
				if !seen[key] {
					answerSpans = append(answerSpans, Span{a.Manifest.DocumentSHA256, blockID, start, start + utf8.RuneCountInString(quote), quote})
					seen[key] = true
				}
				return nil
			}
			for _, f := range card.Facts {
				if err := appendQuote(f.BlockID, f.Quote); err != nil {
					return err
				}
			}
			required := append([]string{}, card.Conditions...)
			required = append(required, card.RequiredItems...)
			required = append(required, card.ExactCode)
			required = append(required, card.OrderedActions...)
			required = append(required, card.Result)
			for _, phrase := range required {
				if phrase == "" {
					continue
				}
				found := false
				for _, id := range card.BlockIDs {
					if strings.Contains(byID[id].Text, phrase) {
						if err := appendQuote(id, phrase); err != nil {
							return err
						}
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("unsupported answer field")
				}
			}
			compact := []string{}
			for _, sp := range answerSpans {
				compact = append(compact, sp.Quote)
			}
			answer = strings.Join(compact, "\n\n")
			if len(card.OrderedActions) > 0 {
				steps := []string{}
				for i, action := range card.OrderedActions {
					steps = append(steps, fmt.Sprintf("%d. %s", i+1, action))
				}
				answer += "\n\nПорядок действий:\n" + strings.Join(steps, "\n")
			}
		}
		// Keep the exact evidence in answers; annotations enrich search only.
		context := []string{a.Manifest.GameName, card.Episode, card.Branch, card.Location, card.Task}
		context = append(context, card.Conditions...)
		context = append(context, card.RequiredItems...)
		context = append(context, card.Aliases...)
		parts := []string{}
		for _, s := range context {
			if s != "" {
				parts = append(parts, s)
			}
		}
		a.Entries = append(a.Entries, Entry{card.ID, card.Episode, strings.Join(parts, "\n") + "\n\n" + strings.Join(quotes, "\n\n"), answer, answerSpans})
	}
	return nil
}
func Load(dir string) (Artifacts, error) {
	var a Artifacts
	for name, v := range map[string]any{"manifest.yaml": &a.Manifest, "blocks.yaml": &a.Blocks, "cards.yaml": &a.Cards} {
		b, e := os.ReadFile(filepath.Join(dir, name))
		if e != nil {
			return a, e
		}
		if e = decode(b, v); e != nil {
			return a, fmt.Errorf("%s: %w", name, e)
		}
	}
	var e error
	a.Raw, e = os.ReadFile(filepath.Join(dir, "source.bin"))
	if e != nil {
		return a, e
	}
	doc, e := os.ReadFile(filepath.Join(dir, "document.txt"))
	if e != nil {
		return a, e
	}
	a.Document = string(doc)
	// Validate stored evidence before deriving projections; never silently repair it.
	if e = validate(a, false); e != nil {
		return a, e
	}
	e = hydrate(&a)
	return a, e
}
func validate(a Artifacts, review bool) error {
	m := a.Manifest
	if m.Version != Version || !slug.MatchString(m.GameID) || m.GameName == "" || hash(a.Raw) != m.RawSHA256 || hash([]byte(a.Document)) != m.DocumentSHA256 {
		return fmt.Errorf("invalid manifest or source hash mismatch")
	}
	if len(a.Blocks) == 0 || len(a.Cards) == 0 {
		return fmt.Errorf("empty blocks/cards")
	}
	runes := []rune(a.Document)
	byID := map[string]Block{}
	order := []string{}
	last := 0
	for _, b := range a.Blocks {
		expectedStart := last
		if len(order) > 0 {
			expectedStart += 2
		}
		if b.ID == "" || b.Start != expectedStart || b.Start < 0 || b.End > len(runes) || b.End <= b.Start || string(runes[b.Start:b.End]) != b.Text {
			return fmt.Errorf("invalid source block %s", b.ID)
		}
		if _, ok := byID[b.ID]; ok {
			return fmt.Errorf("duplicate block %s", b.ID)
		}
		byID[b.ID] = b
		order = append(order, b.ID)
		last = b.End
	}
	if last != len(runes) {
		return fmt.Errorf("blocks omit source text")
	}
	rebuilt := []string{}
	for _, b := range a.Blocks {
		rebuilt = append(rebuilt, b.Text)
	}
	if strings.Join(rebuilt, "\n\n") != a.Document {
		return fmt.Errorf("blocks do not reconstruct document")
	}
	assigned := []string{}
	seen := map[string]bool{}
	for _, c := range a.Cards {
		if !slug.MatchString(c.ID) || seen[c.ID] || strings.TrimSpace(c.Task) == "" || len(c.BlockIDs) == 0 {
			return fmt.Errorf("invalid/duplicate card %s", c.ID)
		}
		seen[c.ID] = true
		if m.AnnotationProvider != "" && c.ValidationStatus == "source_only" {
			return fmt.Errorf("AI card %s cannot be source_only", c.ID)
		}
		switch c.ValidationStatus {
		case "source_only", "reviewed":
		case "needs_review":
			if review {
				return fmt.Errorf("card %s needs semantic review; inspect cards.yaml and mark reviewed", c.ID)
			}
		default:
			return fmt.Errorf("unknown validation status for %s", c.ID)
		}
		assigned = append(assigned, c.BlockIDs...)
		evidence := []string{}
		if len(c.SourceSpans) != len(c.BlockIDs) {
			return fmt.Errorf("card %s: evidence coverage mismatch", c.ID)
		}
		for i, id := range c.BlockIDs {
			b, ok := byID[id]
			if !ok {
				return fmt.Errorf("unknown block %s", id)
			}
			s := c.SourceSpans[i]
			if s.BlockID != id || s.DocumentSHA256 != m.DocumentSHA256 || s.Start != b.Start || s.End != b.End || s.Quote != b.Text {
				return fmt.Errorf("card %s: source span mismatch", c.ID)
			}
			evidence = append(evidence, b.Text)
		}
		text := strings.Join(evidence, "\n\n")
		fields := []string{c.Branch, c.Location, c.ExactCode, c.Result}
		fields = append(fields, c.Conditions...)
		fields = append(fields, c.RequiredItems...)
		fields = append(fields, c.OrderedActions...)
		if c.ValidationStatus != "source_only" {
			fields = append(fields, c.Task)
			if c.Episode != "" && !strings.Contains(text, c.Episode) { // inherited hierarchy is also valid context
				found := false
				for _, id := range c.BlockIDs {
					if strings.Join(byID[id].HeadingPath, " / ") == c.Episode {
						found = true
					}
				}
				if !found {
					return fmt.Errorf("card %s: episode absent from source", c.ID)
				}
			}
		}
		for _, f := range fields {
			if f != "" && !strings.Contains(text, f) {
				return fmt.Errorf("card %s: field is not an exact source phrase: %q", c.ID, f)
			}
		}
		for _, f := range c.Facts {
			b, ok := byID[f.BlockID]
			assignedHere := false
			for _, id := range c.BlockIDs {
				if id == f.BlockID {
					assignedHere = true
				}
			}
			if !ok || !assignedHere || f.Quote == "" || !strings.Contains(b.Text, f.Quote) {
				return fmt.Errorf("card %s: unsupported fact", c.ID)
			}
		}
	}
	if !reflect.DeepEqual(order, assigned) {
		return fmt.Errorf("cards must cover every source block exactly once in source order")
	}
	return nil
}
func Validate(c Config) error {
	a, e := Load(c.Artifacts)
	if e != nil {
		return e
	}
	return validate(a, c.RequireReview)
}
