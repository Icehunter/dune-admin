package main

import "testing"

func TestSampleTableQuery(t *testing.T) {
	t.Parallel()

	origSchema := dbSchema
	t.Cleanup(func() { dbSchema = origSchema })
	dbSchema = "dune"

	query := sampleTableQuery(`items"; DROP TABLE dune.items; --`, 25)
	want := `SELECT * FROM "dune"."items""; DROP TABLE dune.items; --" LIMIT 25`
	if query != want {
		t.Fatalf("unexpected query sanitization\nwant: %q\ngot:  %q", want, query)
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
