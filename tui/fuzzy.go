package tui

import (
	"sort"
	"strings"
	"unicode"
)

type fuzzyItem struct {
	Label  string
	Detail string
	Row    int
}

type fuzzyMatch struct {
	Item  fuzzyItem
	Index int
	Score int
}

type fuzzyFinder struct {
	Title  string
	Query  string
	Items  []fuzzyItem
	Cursor int
	Scroll int
}

func newFuzzyFinder(title string, items []fuzzyItem) *fuzzyFinder {
	return &fuzzyFinder{
		Title: title,
		Items: append([]fuzzyItem(nil), items...),
	}
}

func (f *fuzzyFinder) SetQuery(query string) {
	f.Query = query
	f.Cursor = 0
	f.Scroll = 0
}

func (f *fuzzyFinder) Backspace() bool {
	if f.Query == "" {
		return false
	}
	runes := []rune(f.Query)
	f.SetQuery(string(runes[:len(runes)-1]))
	return true
}

func (f *fuzzyFinder) Insert(text string) {
	if text == "" {
		return
	}
	f.SetQuery(f.Query + text)
}

func (f *fuzzyFinder) Move(delta int) {
	matches := f.Matches()
	if len(matches) == 0 {
		f.Cursor = 0
		f.Scroll = 0
		return
	}
	f.Cursor += delta
	if f.Cursor < 0 {
		f.Cursor = 0
	}
	if f.Cursor >= len(matches) {
		f.Cursor = len(matches) - 1
	}
}

func (f *fuzzyFinder) Selected() (fuzzyItem, bool) {
	matches := f.Matches()
	if len(matches) == 0 || f.Cursor < 0 || f.Cursor >= len(matches) {
		return fuzzyItem{}, false
	}
	return matches[f.Cursor].Item, true
}

func (f *fuzzyFinder) Matches() []fuzzyMatch {
	query := strings.TrimSpace(f.Query)
	matches := make([]fuzzyMatch, 0, len(f.Items))
	for index, item := range f.Items {
		score, ok := fuzzyScore(item.Label, query)
		if !ok {
			continue
		}
		matches = append(matches, fuzzyMatch{Item: item, Index: index, Score: score})
	}
	sort.SliceStable(matches, func(i int, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score < matches[j].Score
		}
		return matches[i].Index < matches[j].Index
	})
	return matches
}

func (f *fuzzyFinder) EnsureCursorVisible(visibleRows int) {
	if visibleRows <= 0 {
		f.Scroll = 0
		return
	}
	matches := f.Matches()
	if len(matches) == 0 {
		f.Cursor = 0
		f.Scroll = 0
		return
	}
	if f.Cursor >= len(matches) {
		f.Cursor = len(matches) - 1
	}
	if f.Cursor < f.Scroll {
		f.Scroll = f.Cursor
	}
	if f.Cursor >= f.Scroll+visibleRows {
		f.Scroll = f.Cursor - visibleRows + 1
	}
	maxScroll := len(matches) - visibleRows
	if maxScroll < 0 {
		maxScroll = 0
	}
	if f.Scroll > maxScroll {
		f.Scroll = maxScroll
	}
	if f.Scroll < 0 {
		f.Scroll = 0
	}
}

func fuzzyScore(candidate string, query string) (int, bool) {
	if query == "" {
		return 0, true
	}
	candidateRunes := []rune(strings.ToLower(candidate))
	score := 0
	last := -1
	for _, q := range strings.ToLower(query) {
		if unicode.IsSpace(q) {
			continue
		}
		found := -1
		for index := last + 1; index < len(candidateRunes); index++ {
			if candidateRunes[index] == q {
				found = index
				break
			}
		}
		if found < 0 {
			return 0, false
		}
		if last < 0 {
			score += found * 8
		} else {
			score += found - last - 1
		}
		last = found
	}
	return score, true
}
