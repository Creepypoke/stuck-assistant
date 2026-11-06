package query

import (
	"testing"

	"github.com/walkthrough-assistance/backend/storage/rag"
)

func TestCleanEvidenceRemovesMachineText(t *testing.T) {
	in := "CHAPTER 5 | third-choice/Gail\nГлава 5.\nenter the password 31415 and choose Yes [evidence:baseline-block-033:27038-27180]\nExact code: 31415"
	want := "CHAPTER 5\nГлава 5.\nenter the password 31415 and choose Yes\nExact code: 31415"
	if got := cleanEvidence(in); got != want {
		t.Fatalf("got %q", got)
	}
	// Readable location fields stay.
	if got := cleanEvidence("CHAPTER 3 | infirmary / next room"); got != "CHAPTER 3 | infirmary / next room" {
		t.Fatalf("location removed: %q", got)
	}
}

func TestModelSectionHidesSituationID(t *testing.T) {
	if s := modelSection(rag.SearchResult{SituationID: "menu-mix", Section: "menu-mix"}); s != "" {
		t.Fatalf("situation ID leaked: %q", s)
	}
	if s := modelSection(rag.SearchResult{Section: "CHAPTER 3 – MISSION ACCOMPLISHED"}); s != "CHAPTER 3 – MISSION ACCOMPLISHED" {
		t.Fatalf("plain chunk section lost: %q", s)
	}
}
