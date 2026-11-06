package rag

import (
	"encoding/json"
	"testing"
)

func TestSituationSourceAndAnswerSeparation(t *testing.T) {
	md := "Перед. Чип B2: 0392. После."
	quote := "Чип B2: 0392."
	span := SourceSpan{DocumentSHA256: DocumentHash(md), BlockID: "b2", Start: 7, End: 20, Quote: quote}
	data, _ := json.Marshal(map[string]any{"situation": Situation{Schema: "situation-v1", ID: "b2-chip", AnswerText: quote, SourceSpans: []SourceSpan{span}}})
	r := SearchResult{Text: "Dino Crisis B2 чип цифровой пароль. " + quote}
	if err := ApplySituation(&r, data); err != nil {
		t.Fatal(err)
	}
	if r.Text == r.AnswerText || r.SituationID != "b2-chip" {
		t.Fatal("representations were lost")
	}
	if err := ValidateSourceSpans(r, md); err != nil {
		t.Fatal(err)
	}
	r.SourceSpans[0].Start = 0
	if ValidateSourceSpans(r, md) == nil {
		t.Fatal("wrong episode offset was accepted")
	}
}

func TestSituationExactCodeMustBeQuoted(t *testing.T) {
	quote := "enter the password 3695"
	span := SourceSpan{DocumentSHA256: "h", BlockID: "b1", Start: 0, End: len([]rune(quote)), Quote: quote}
	situation := func(code string) []byte {
		data, _ := json.Marshal(map[string]any{"situation": Situation{Schema: "situation-v1", ID: "b1", AnswerText: quote, SourceSpans: []SourceSpan{span}, ExactCode: []string{code}}})
		return data
	}
	r := SearchResult{Text: quote}
	if err := ApplySituation(&r, situation("3695")); err != nil || len(r.ExactCode) != 1 {
		t.Fatalf("quoted code rejected: %v", err)
	}
	r = SearchResult{Text: quote}
	if ApplySituation(&r, situation("0392")) == nil {
		t.Fatal("code absent from the source was accepted")
	}
}

func TestRejectUnknownSituationSchema(t *testing.T) {
	var r SearchResult
	if ApplySituation(&r, []byte(`{"situation":{"schema":"future"}}`)) == nil {
		t.Fatal("unsupported schema accepted")
	}
	if err := ApplySituation(&r, []byte(`{"token_estimate":10}`)); err != nil {
		t.Fatal(err)
	}
}
