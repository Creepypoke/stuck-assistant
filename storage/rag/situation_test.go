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

func TestRejectUnknownSituationSchema(t *testing.T) {
	var r SearchResult
	if ApplySituation(&r, []byte(`{"situation":{"schema":"future"}}`)) == nil {
		t.Fatal("unsupported schema accepted")
	}
	if err := ApplySituation(&r, []byte(`{"token_estimate":10}`)); err != nil {
		t.Fatal(err)
	}
}
