package diff

import (
	"fmt"
	"strings"
	"testing"
)

func TestCompareKeepsHunksAndLineNumbers(t *testing.T) {
	var shared strings.Builder
	for i := range 3000 {
		fmt.Fprintf(&shared, "%04d\n", i)
	}
	before := "old first\n" + shared.String() + "old last\n"
	after := "new first\ninserted\n" + shared.String() + "new last\n"
	got := Compare(before, after)
	if got.Added != 3 || got.Removed != 2 {
		t.Fatalf("expected both distant edits, got +%d -%d", got.Added, got.Removed)
	}
	if len(got.Rows) != 12 || got.Rows[6].Omitted != 2994 {
		t.Fatalf("expected two hunks with three context lines each: %+v", got.Rows)
	}
	deleted, added := got.Rows[len(got.Rows)-2], got.Rows[len(got.Rows)-1]
	if deleted.Kind != "-" || deleted.Line != 3002 || added.Kind != "+" || added.Line != 3003 {
		t.Fatalf("unexpected old/new line numbers: %+v / %+v", deleted, added)
	}
}

func TestComparePreservesLineEndingsAndUnicode(t *testing.T) {
	got := Compare("你好\r\n世界", "你好\r\n世界\n")
	if got.Added != 1 || got.Removed != 1 || len(got.Rows) != 3 {
		t.Fatalf("unexpected diff: %+v", got)
	}
	if got.Rows[0].Text != "你好\r" || !got.Rows[1].NoNewline || got.Rows[2].NoNewline {
		t.Fatalf("unexpected line endings: %+v", got.Rows)
	}
	if unchanged := Compare("你好\n", "你好\n"); len(unchanged.Rows) != 0 || unchanged.Added+unchanged.Removed != 0 {
		t.Fatalf("identical text generated rows: %+v", unchanged)
	}
}

func TestCompareManyDistinctLineTokens(t *testing.T) {
	var before, after strings.Builder
	for i := range 60000 {
		line := fmt.Sprintf("%d\n", i)
		before.WriteString(line)
		after.WriteString(line)
	}
	after.WriteString("末尾")
	got := Compare(before.String(), after.String())
	if got.Added != 1 || got.Removed != 0 || len(got.Rows) != 4 {
		t.Fatalf("unexpected diff across surrogate boundary: %+v", got)
	}
	row := got.Rows[len(got.Rows)-1]
	if row.Line != 60001 || row.Text != "末尾" || !row.NoNewline {
		t.Fatalf("unexpected last line: %+v", row)
	}
}
