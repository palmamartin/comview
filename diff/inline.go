package diff

import (
	"unicode"
	"unicode/utf8"
)

type token struct {
	text       string
	start, end int
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

	pairs := len(deletes)
	if len(adds) < pairs {
		pairs = len(adds)
	}

	for i := 0; i < pairs; i++ {
		_, oldCode := splitLine(deletes[i])
		_, newCode := splitLine(adds[i])
		diffs.deleteSpans[i], diffs.addSpans[i] = inlineSpans(oldCode, newCode)
	}

	return diffs
}

func inlineSpans(oldCode string, newCode string) ([]InlineSpan, []InlineSpan) {
	oldTokens := tokenizeInline(oldCode)
	newTokens := tokenizeInline(newCode)
	oldMatched, newMatched := matchedTokens(oldTokens, newTokens)

	return changedSpans(oldTokens, oldMatched), changedSpans(newTokens, newMatched)
}

func matchedTokens(oldTokens []token, newTokens []token) ([]bool, []bool) {
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

	oldMatched := make([]bool, len(oldTokens))
	newMatched := make([]bool, len(newTokens))
	for i, j := 0, 0; i < len(oldTokens) && j < len(newTokens); {
		if oldTokens[i].text == newTokens[j].text {
			oldMatched[i] = true
			newMatched[j] = true
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
		if span.Start <= last.End || onlyWhitespaceBetween(tokens, last.End, span.Start) {
			if span.End > last.End {
				last.End = span.End
			}
			continue
		}
		merged = append(merged, span)
	}
	return merged
}

func onlyWhitespaceBetween(tokens []token, start int, end int) bool {
	for _, tok := range tokens {
		if tok.start >= end {
			return true
		}
		if tok.end <= start {
			continue
		}
		return false
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
		for end < len(text) {
			next, size := runeAt(text, end)
			if isSpace(next) || tokenKind(next) != kind {
				break
			}
			end += size
		}
		tokens = append(tokens, token{
			text:  text[start:end],
			start: start,
			end:   end,
		})
		start = end
	}
	return tokens
}

func tokenKind(r rune) int {
	switch {
	case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_':
		return 1
	default:
		return 2
	}
}

func isSpace(r rune) bool {
	return unicode.IsSpace(r)
}

func runeAt(text string, index int) (rune, int) {
	return utf8.DecodeRuneInString(text[index:])
}
