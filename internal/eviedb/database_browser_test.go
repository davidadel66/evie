package eviedb

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestDatabaseCellRedactsAndBoundsRenderedValues(t *testing.T) {
	secret := databaseCell("do-not-expose", true)
	if !secret.Redacted || secret.Kind != "redacted" || secret.Value != nil {
		t.Fatalf("redacted cell=%+v", secret)
	}

	long := databaseCell(strings.Repeat("é", maxDatabaseCellRunes+1), false)
	if !long.Truncated || long.Value == nil || utf8.RuneCountInString(*long.Value) != maxDatabaseCellRunes+1 || !strings.HasSuffix(*long.Value, "…") {
		t.Fatalf("bounded cell=%+v", long)
	}
}
