package main

import "testing"

func TestSampleTableQuery(t *testing.T) {
	t.Parallel()

	query := sampleTableQuery(`items"; DROP TABLE dune.items; --`, 25)
	want := `SELECT * FROM "items""; DROP TABLE dune.items; --" LIMIT 25`
	if query != want {
		t.Fatalf("unexpected query sanitization\nwant: %q\ngot:  %q", want, query)
	}
}

// TestSampleTableQuery_IgnoresEmptyPackageLevelSchema is a regression test for
// #283: the package-level dbSchema global is empty at runtime after the
// multi-server refactor, which used to produce `SELECT * FROM "".tbl LIMIT n`
// — a Postgres "zero-length delimited identifier" error (SQLSTATE 42601).
// sampleTableQuery must resolve the table through the connection's
// search_path (a bare identifier) instead of prefixing the empty global.
func TestSampleTableQuery_IgnoresEmptyPackageLevelSchema(t *testing.T) {
	t.Parallel()

	origSchema := dbSchema
	t.Cleanup(func() { dbSchema = origSchema })
	dbSchema = ""

	query := sampleTableQuery("player_state", 20)
	want := `SELECT * FROM "player_state" LIMIT 20`
	if query != want {
		t.Fatalf("unexpected query with empty dbSchema\nwant: %q\ngot:  %q", want, query)
	}
}

func TestFormatSampleRow(t *testing.T) {
	t.Parallel()

	row := formatSampleRow([]any{int64(1), "alpha", nil, true})
	want := []string{"1", "alpha", "<nil>", "true"}
	if len(row) != len(want) {
		t.Fatalf("unexpected row length: got %d want %d", len(row), len(want))
	}
	for i := range want {
		if row[i] != want[i] {
			t.Fatalf("unexpected row[%d]: got %q want %q", i, row[i], want[i])
		}
	}
}
