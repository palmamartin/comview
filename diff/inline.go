package diff

import (
	"unicode/utf8"

	"github.com/rockorager/go-uucode"
)

type token struct {
	text       string
	start, end int
	kind       int
}

type inlineDiffs struct {
	deleteSpans [][]InlineSpan
	addSpans    [][]InlineSpan
}

func pairInlineDiffs(deletes []Line, adds []Line) inlineDiffs {
	diffs := inlineDiffs{
		deleteSpans: make([][]InlineSpan, len(deletes)),
		addSpans:    make([][]InlineSpan, len(adds)),
	}

	for _, pair := range inlineLinePairs(deletes, adds) {
		_, oldCode := splitLine(deletes[pair.deleteIndex])
		_, newCode := splitLine(adds[pair.addIndex])
		deleteSpans, addSpans := inlineSpans(oldCode, newCode)
		diffs.deleteSpans[pair.deleteIndex] = deleteSpans
		diffs.addSpans[pair.addIndex] = addSpans
	}

	return diffs
}

type inlineLinePair struct {
	deleteIndex int
	addIndex    int
}

const (
	minInlineLineSimilarity = 0.45
	leadingTokenMatchBonus  = 0.5
)

func inlineLinePairs(deletes []Line, adds []Line) []inlineLinePair {
	if len(deletes) == 0 || len(adds) == 0 {
		return nil
	}

	scores := make([][]float64, len(deletes))
	for i := range deletes {
		scores[i] = make([]float64, len(adds))
		_, oldCode := splitLine(deletes[i])
		for j := range adds {
			_, newCode := splitLine(adds[j])
			scores[i][j] = lineSimilarity(oldCode, newCode)
		}
	}

	dp := make([][]float64, len(deletes)+1)
	for i := range dp {
		dp[i] = make([]float64, len(adds)+1)
	}

	for i := len(deletes) - 1; i >= 0; i-- {
		for j := len(adds) - 1; j >= 0; j-- {
			best := dp[i+1][j]
			if dp[i][j+1] > best {
				best = dp[i][j+1]
			}
			if scores[i][j] >= minInlineLineSimilarity && scores[i][j]+dp[i+1][j+1] > best {
				best = scores[i][j] + dp[i+1][j+1]
			}
			dp[i][j] = best
		}
	}

	pairs := make([]inlineLinePair, 0)
	for i, j := 0, 0; i < len(deletes) && j < len(adds); {
		pairScore := scores[i][j] + dp[i+1][j+1]
		if scores[i][j] >= minInlineLineSimilarity && pairScore >= dp[i+1][j] && pairScore >= dp[i][j+1] {
			pairs = append(pairs, inlineLinePair{
				deleteIndex: i,
				addIndex:    j,
			})
			i++
			j++
			continue
		}
		if dp[i+1][j] >= dp[i][j+1] {
			i++
		} else {
			j++
		}
	}

	return pairs
}

func inlineSpans(oldCode string, newCode string) ([]InlineSpan, []InlineSpan) {
	oldTokens := tokenizeInline(oldCode)
	newTokens := tokenizeInline(newCode)
	oldMatched, newMatched := matchedTokens(oldTokens, newTokens)

	return changedSpans(oldTokens, oldMatched), changedSpans(newTokens, newMatched)
}

func lineSimilarity(oldCode string, newCode string) float64 {
	oldTokens := tokenizeInline(oldCode)
	newTokens := tokenizeInline(newCode)
	oldSimilarityTokens := similarityTokens(oldTokens)
	newSimilarityTokens := similarityTokens(newTokens)
	if len(oldSimilarityTokens) > 0 && len(newSimilarityTokens) > 0 {
		oldTokens = oldSimilarityTokens
		newTokens = newSimilarityTokens
	}
	if len(oldTokens) == 0 || len(newTokens) == 0 {
		if oldCode == newCode {
			return 1
		}
		return 0
	}

	common := matchedTokenCount(oldTokens, newTokens)
	similarity := float64(2*common) / float64(len(oldTokens)+len(newTokens))
	if oldTokens[0].text == newTokens[0].text {
		similarity += leadingTokenMatchBonus
		if similarity > 1 {
			similarity = 1
		}
	}
	return similarity
}

func similarityTokens(tokens []token) []token {
	filtered := make([]token, 0, len(tokens))
	for _, tok := range tokens {
		if tok.kind == wordToken {
			filtered = append(filtered, tok)
		}
	}
	return filtered
}

func matchedTokenCount(oldTokens []token, newTokens []token) int {
	return tokenLCSMatrix(oldTokens, newTokens)[0][0]
}

func tokenLCSMatrix(oldTokens []token, newTokens []token) [][]int {
	dp := make([][]int, len(oldTokens)+1)
	for i := range dp {
		dp[i] = make([]int, len(newTokens)+1)
	}

	for i := len(oldTokens) - 1; i >= 0; i-- {
		for j := len(newTokens) - 1; j >= 0; j-- {
			if oldTokens[i].text == newTokens[j].text {
				dp[i][j] = dp[i+1][j+1] + 1
				continue
			}
			if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}
	return dp
}

func matchedTokens(oldTokens []token, newTokens []token) ([]bool, []bool) {
	oldMatched := make([]bool, len(oldTokens))
	newMatched := make([]bool, len(newTokens))
	prefix := 0
	for prefix < len(oldTokens) && prefix < len(newTokens) && oldTokens[prefix].text == newTokens[prefix].text {
		oldMatched[prefix] = true
		newMatched[prefix] = true
		prefix++
	}

	oldSuffix := len(oldTokens) - 1
	newSuffix := len(newTokens) - 1
	for oldSuffix >= prefix && newSuffix >= prefix && oldTokens[oldSuffix].text == newTokens[newSuffix].text {
		oldMatched[oldSuffix] = true
		newMatched[newSuffix] = true
		oldSuffix--
		newSuffix--
	}

	middleOldTokens := oldTokens[prefix : oldSuffix+1]
	middleNewTokens := newTokens[prefix : newSuffix+1]
	dp := tokenLCSMatrix(middleOldTokens, middleNewTokens)
	for i, j := 0, 0; i < len(oldTokens) && j < len(newTokens); {
		if i < prefix || i > oldSuffix {
			i++
			continue
		}
		if j < prefix || j > newSuffix {
			j++
			continue
		}
		middleI := i - prefix
		middleJ := j - prefix
		if middleOldTokens[middleI].text == middleNewTokens[middleJ].text {
			oldMatched[i] = true
			newMatched[j] = true
			i++
			j++
			continue
		}
		if dp[middleI+1][middleJ] >= dp[middleI][middleJ+1] {
			i++
		} else {
			j++
		}
	}

	return oldMatched, newMatched
}

func changedSpans(tokens []token, matched []bool) []InlineSpan {
	spans := make([]InlineSpan, 0)
	for i, tok := range tokens {
		if matched[i] {
			continue
		}
		spans = append(spans, InlineSpan{
			Start: tok.start,
			End:   tok.end,
			Kind:  InlineChange,
		})
	}
	return mergeInlineSpans(tokens, spans)
}

func mergeInlineSpans(tokens []token, spans []InlineSpan) []InlineSpan {
	if len(spans) == 0 {
		return nil
	}

	merged := []InlineSpan{spans[0]}
	for _, span := range spans[1:] {
		last := &merged[len(merged)-1]
		if span.Start <= last.End || shouldCoalesceInlineGap(tokens, last.End, span.Start) {
			if span.End > last.End {
				last.End = span.End
			}
			continue
		}
		merged = append(merged, span)
	}
	return merged
}

func shouldCoalesceInlineGap(tokens []token, start int, end int) bool {
	const maxGapBytes = 12

	if end-start > maxGapBytes {
		return false
	}

	gapTokens := 0
	for _, tok := range tokens {
		if tok.start >= end {
			return true
		}
		if tok.end <= start {
			continue
		}
		gapTokens++
		if gapTokens > 2 {
			return false
		}
	}
	return true
}

func tokenizeInline(text string) []token {
	tokens := make([]token, 0)
	for start := 0; start < len(text); {
		r, size := utf8.DecodeRuneInString(text[start:])
		if isSpace(r) {
			start += size
			continue
		}
		end := start + size
		kind := tokenKind(r)
		if kind == wordToken {
			for end < len(text) {
				next, size := runeAt(text, end)
				if isSpace(next) || tokenKind(next) != kind {
					break
				}
				end += size
			}
		}
		tokens = append(tokens, token{
			text:  text[start:end],
			start: start,
			end:   end,
			kind:  kind,
		})
		start = end
	}
	return tokens
}

const (
	wordToken = iota + 1
	punctuationToken
	symbolToken
)

func tokenKind(r rune) int {
	switch {
	case uucode.IsLetter(r) || uucode.IsDigit(r) || r == '_':
		return wordToken
	case uucode.IsPunct(r):
		return punctuationToken
	default:
		return symbolToken
	}
}

func isSpace(r rune) bool {
	return uucode.IsSpace(r)
}

func runeAt(text string, index int) (rune, int) {
	return utf8.DecodeRuneInString(text[index:])
}
