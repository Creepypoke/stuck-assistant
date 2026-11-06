package rag

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// Reciprocal rank fusion combines ranks rather than incomparable BM25/cosine
// scores. Each list contributes at most once per chunk. The vector-only order
// is retained if the lexical query is empty or has no matches.
func fuseKeywords(ctx context.Context, db *sql.DB, semantic []rankedChunk, question string, limit int) ([]rankedChunk, error) {
	expression := keywordExpression(question)
	if expression == "" || len(semantic) == 0 {
		return semantic, nil
	}
	candidates := max(20, limit*4)
	rows, err := db.QueryContext(ctx, `SELECT rowid, bm25(chunks_fts, 1.0, 2.0, 1.0) FROM chunks_fts WHERE chunks_fts MATCH ?
 ORDER BY bm25(chunks_fts, 1.0, 2.0, 1.0), rowid LIMIT ?`, expression, candidates)
	if err != nil {
		return nil, fmt.Errorf("search FTS index: %w", err)
	}
	defer rows.Close()
	keywordRanks := make(map[int64]int)
	keywordScores := make(map[int64]float64)
	for rows.Next() {
		var id int64
		var score float64
		if err := rows.Scan(&id, &score); err != nil {
			return nil, fmt.Errorf("read FTS match: %w", err)
		}
		keywordRanks[id] = len(keywordRanks) + 1
		keywordScores[id] = score
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate FTS matches: %w", err)
	}
	if len(keywordRanks) == 0 {
		return semantic, nil
	}
	const rankConstant = 60.0
	fused := make([]rankedChunk, 0, min(len(semantic), candidates*2))
	for i, chunk := range semantic {
		if i < candidates {
			chunk.fusion = 1 / (rankConstant + float64(i+1))
		}
		if rank, ok := keywordRanks[chunk.ChunkID]; ok {
			chunk.fusion += 1 / (rankConstant + float64(rank))
			score := keywordScores[chunk.ChunkID]
			chunk.BM25 = &score
		}
		if chunk.fusion > 0 {
			score := chunk.fusion
			chunk.RRF = &score
			fused = append(fused, chunk)
		}
	}
	sort.Slice(fused, func(i, j int) bool {
		if fused[i].fusion != fused[j].fusion {
			return fused[i].fusion > fused[j].fusion
		}
		// Semantic evidence breaks fusion ties; source order makes equal scores stable.
		if fused[i].Score != fused[j].Score {
			return fused[i].Score > fused[j].Score
		}
		return earlier(fused[i], fused[j])
	})
	return fused, nil
}

// Tokenize plain user text instead of accepting FTS query syntax. Quoting every
// token prevents operators, quotes, column selectors and punctuation from
// becoming an executable MATCH expression. unicode61 handles both corpus languages.
func keywordExpression(question string) string {
	tokens := strings.FieldsFunc(strings.ToLower(question), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	seen := make(map[string]bool)
	terms := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if seen[token] || keywordStopWords[token] {
			continue
		}
		seen[token] = true
		terms = append(terms, `"`+token+`"`)
		if len(terms) == 64 {
			break
		}
	}
	return strings.Join(terms, " OR ")
}

var keywordStopWords = func() map[string]bool {
	words := strings.Fields(`a an the and or of to in on at for from with is are do does did i we you my me it this that what which where how can should please
 я мы ты вы мне меня мой моя у в во на с со из к ко по за от до и или а но это что как где куда какой какая какие какое нужно надо сделать делать чтобы после перед затем только сейчас сразу пожалуйста без`)
	result := make(map[string]bool, len(words))
	for _, word := range words {
		result[word] = true
	}
	return result
}()
