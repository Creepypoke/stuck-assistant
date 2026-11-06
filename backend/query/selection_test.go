package query

import (
	"strings"
	"testing"

	"github.com/walkthrough-assistance/backend/storage/rag"
)

func TestCutRelativeKeepsCloseSourcesInRankOrder(t *testing.T) {
	results := []rag.SearchResult{{ChunkID: 1, Score: 0.60}, {ChunkID: 2, Score: 0.40}, {ChunkID: 3, Score: 0.66}, {ChunkID: 4, Score: 0.55}}
	got := cutRelative(results, 0.85)
	ids := []int64{}
	for _, r := range got {
		ids = append(ids, r.ChunkID)
	}
	// Best cosine is 0.66 (rank 3), so the bar is 0.561: rank 1 always stays,
	// 0.40 and 0.55 are dropped.
	if len(ids) != 2 || ids[0] != 1 || ids[1] != 3 {
		t.Fatalf("unexpected selection %v", ids)
	}
	if len(cutRelative(results, 0)) != 4 {
		t.Fatal("zero cutoff must keep every result")
	}
}

func codeSource(id, code, quote string) rag.SearchResult {
	return rag.SearchResult{SituationID: id, ExactCode: []string{code}, SourceSpans: []rag.SourceSpan{{Quote: quote}}}
}

func TestCodeAnswerUsesVerifiedCardCode(t *testing.T) {
	top := codeSource("b1", "3695", "use the B1 Key Chip on it and enter the password 3695")
	answer, ok := codeAnswer(Request{Question: "Какой пароль ввести для B1 Key Chip?"}, []rag.SearchResult{top, {SituationID: "other"}})
	if !ok || !strings.Contains(answer, "**3695**") || !strings.Contains(answer, "enter the password 3695") {
		t.Fatalf("unexpected answer %q ok=%v", answer, ok)
	}
	answer, ok = codeAnswer(Request{Question: "What is the B1 chip code?"}, []rag.SearchResult{top})
	if !ok || !strings.HasPrefix(answer, "Code: **3695**") {
		t.Fatalf("unexpected English answer %q", answer)
	}
}

func TestCodeAnswerFallsBackToModel(t *testing.T) {
	b1 := codeSource("b1", "3695", "enter the password 3695")
	office := codeSource("office", "58104", "enter the password 58104")
	cases := map[string]struct {
		question string
		results  []rag.SearchResult
	}{
		"not a code question":         {"Где взять дробовик?", []rag.SearchResult{b1}},
		"another source differs":      {"Какой пароль компьютера?", []rag.SearchResult{b1, office}},
		"top source has no code":      {"Какой код нужен?", []rag.SearchResult{{SituationID: "x"}, b1}},
		"code missing from the quote": {"Какой код?", []rag.SearchResult{codeSource("y", "1111", "no digits here")}},
	}
	for name, c := range cases {
		if answer, ok := codeAnswer(Request{Question: c.question}, c.results); ok {
			t.Errorf("%s: expected model fallback, got %q", name, answer)
		}
	}
}
