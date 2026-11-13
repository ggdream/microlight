// Package diff compares text line by line and includes surrounding context.
package diff

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sergi/go-diff/diffmatchpatch"
)

type Lines struct {
	Added   int
	Removed int
	Rows    []Row `json:",omitempty"`
}

type Row struct {
	Kind      string `json:",omitempty"`
	Line      int
	Text      string `json:",omitempty"`
	Omitted   int    `json:",omitempty"`
	NoNewline bool   `json:",omitempty"`
}

const contextLines = 3

// Compare computes a line diff with a bounded search. When the matcher
// reaches its deadline, it can report an unmatched region as a replacement.
func Compare(before, after string) *Lines {
	result := new(Lines)
	if before == after {
		return result
	}

	edits := lineEdits(before, after)
	oldLine, newLine := 1, 1
	for i, edit := range edits {
		switch edit.Type {
		case diffmatchpatch.DiffDelete:
			count := result.appendRows("-", oldLine, edit.Text)
			result.Removed += count
			oldLine += count
		case diffmatchpatch.DiffInsert:
			count := result.appendRows("+", newLine, edit.Text)
			result.Added += count
			newLine += count
		case diffmatchpatch.DiffEqual:
			count := lineCount(edit.Text)
			head, tail := 0, 0
			if i > 0 {
				head = min(contextLines, count)
			}
			if i+1 < len(edits) {
				tail = min(contextLines, count-head)
			}
			for index, line := range lineText(edit.Text) {
				switch {
				case index < head || index >= count-tail:
					result.Rows = append(result.Rows, Row{Line: newLine + index, Text: strings.TrimSuffix(line, "\n")})
				case index == head && head > 0 && tail > 0:
					result.Rows = append(result.Rows, Row{Omitted: count - head - tail})
				}
			}
			oldLine += count
			newLine += count
		}
	}
	return result
}

func lineEdits(before, after string) []diffmatchpatch.Diff {
	// The matcher encodes each distinct line as a Unicode scalar.
	// An upper bound on distinct lines also keeps that encoding lossless.
	const maxLineTokens = utf8.MaxRune - 0x800
	if lineCount(before)+lineCount(after) > maxLineTokens {
		var edits []diffmatchpatch.Diff
		if before != "" {
			edits = append(edits, diffmatchpatch.Diff{Type: diffmatchpatch.DiffDelete, Text: before})
		}
		if after != "" {
			edits = append(edits, diffmatchpatch.Diff{Type: diffmatchpatch.DiffInsert, Text: after})
		}
		return edits
	}

	matcher := diffmatchpatch.New()
	matcher.DiffTimeout = 200 * time.Millisecond
	oldTokens, newTokens, lines := matcher.DiffLinesToRunes(before, after)
	return matcher.DiffCharsToLines(matcher.DiffMainRunes(oldTokens, newTokens, false), lines)
}

func (d *Lines) appendRows(kind string, start int, text string) int {
	count := 0
	for index, line := range lineText(text) {
		d.Rows = append(d.Rows, Row{
			Kind: kind, Line: start + index,
			Text: strings.TrimSuffix(line, "\n"), NoNewline: !strings.HasSuffix(line, "\n"),
		})
		count++
	}
	return count
}

func lineCount(text string) int {
	count := strings.Count(text, "\n")
	if text != "" && !strings.HasSuffix(text, "\n") {
		count++
	}
	return count
}

func lineText(text string) func(func(int, string) bool) {
	return func(yield func(int, string) bool) {
		index := 0
		for line := range strings.SplitAfterSeq(text, "\n") {
			if line == "" || !yield(index, line) {
				return
			}
			index++
		}
	}
}
