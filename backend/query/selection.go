package query

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/walkthrough-assistance/backend/storage/rag"
)

// cutRelative keeps results whose cosine score is at least cutoff × the best
// score in the list. Rank order is preserved and the first result always stays.
func cutRelative(results []rag.SearchResult, cutoff float64) []rag.SearchResult {
	if len(results) == 0 || cutoff <= 0 {
		return results
	}
	best := results[0].Score
	for _, r := range results {
		best = max(best, r.Score)
	}
	kept := results[:1:1]
	for _, r := range results[1:] {
		if r.Score >= cutoff*best {
			kept = append(kept, r)
		}
	}
	return kept
}

var codeQuestion = regexp.MustCompile(`(?i)(код|пароль|password|passcode|\bcode\b|\bpin\b)`)

// codeAnswer returns a fixed answer for a code question when the top source
// carries exactly one source-verified code and no other kept source has a
// different one. Anything else returns ok=false and the model answers instead,
// so ambiguous questions ("which computer?") still reach the model.
func codeAnswer(request Request, results []rag.SearchResult) (answer string, ok bool) {
	if len(results) == 0 || !codeQuestion.MatchString(request.Question) {
		return "", false
	}
	top := results[0]
	if len(top.ExactCode) != 1 || len(top.SourceSpans) == 0 {
		return "", false
	}
	code := top.ExactCode[0]
	for _, other := range results[1:] {
		for _, c := range other.ExactCode {
			if c != code {
				return "", false
			}
		}
	}
	quote := ""
	for _, span := range top.SourceSpans {
		if strings.Contains(span.Quote, code) {
			quote = span.Quote
			break
		}
	}
	if quote == "" {
		return "", false
	}
	if isCyrillic(request.Question) {
		return "Код: **" + code + "**.\n\nИз прохождения: «" + quote + "».", true
	}
	return "Code: **" + code + "**.\n\nFrom the walkthrough: \"" + quote + "\".", true
}

func isCyrillic(text string) bool {
	for _, r := range text {
		if unicode.Is(unicode.Cyrillic, r) {
			return true
		}
	}
	return false
}
