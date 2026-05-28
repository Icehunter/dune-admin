package main

import (
	"strings"
	"testing"
)

func TestFormatSQLRow(t *testing.T) {
	t.Parallel()

	row := formatSQLRow([]any{int64(7), "name", nil})
	if row != "7 │ name │ <nil>" {
		t.Fatalf("unexpected row format: %q", row)
	}
}

func TestBuildSQLResult(t *testing.T) {
	t.Parallel()

	result := buildSQLResult(
		[]string{"id", "name"},
		[][]any{
			{int64(1), "alpha"},
			{int64(2), "beta"},
		},
		false,
	)
	if !strings.Contains(result, "id │ name\n") {
		t.Fatalf("expected header line in result: %q", result)
	}
	if !strings.Contains(result, "1 │ alpha\n") || !strings.Contains(result, "2 │ beta\n") {
		t.Fatalf("expected row lines in result: %q", result)
	}
	if strings.Contains(result, "limited to 200 rows") {
		t.Fatalf("did not expect truncation marker in non-truncated result")
	}

	truncated := buildSQLResult([]string{"id"}, [][]any{{1}}, true)
	if !strings.Contains(truncated, "… (limited to 200 rows)\n") {
		t.Fatalf("expected truncation marker in truncated result: %q", truncated)
	}
}
