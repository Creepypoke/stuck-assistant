package query

import (
	"regexp"
	"strings"

	"github.com/walkthrough-assistance/backend/storage/rag"
)

// The small generator copies machine text from its context into answers, so
// situation cards are cleaned before they reach it. Search text, source spans
// and the stored database are unchanged.

var evidenceMarker = regexp.MustCompile(`[ \t]*\[evidence:[^\]]*\]`)

// A slug such as "third-choice/Gail": no spaces, joined by "-" or "/".
var machineSlug = regexp.MustCompile(`^[a-z0-9]+(?:[-/][A-Za-z0-9]+)+$`)

// cleanEvidence removes [evidence:…] markers and slug fields from
// "CHAPTER 5 | third-choice/Gail" style header lines.
func cleanEvidence(text string) string {
	text = evidenceMarker.ReplaceAllString(text, "")
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if !strings.Contains(line, " | ") {
			continue
		}
		kept := []string{}
		for _, field := range strings.Split(line, " | ") {
			if !machineSlug.MatchString(strings.TrimSpace(field)) {
				kept = append(kept, field)
			}
		}
		lines[i] = strings.Join(kept, " | ")
	}
	return strings.Join(lines, "\n")
}

// modelSection hides a section that is only the situation's machine ID.
func modelSection(r rag.SearchResult) string {
	if r.SituationID != "" && (r.Section == r.SituationID || machineSlug.MatchString(r.Section)) {
		return ""
	}
	return r.Section
}
