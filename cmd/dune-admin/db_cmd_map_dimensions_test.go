package main

import (
	"database/sql"
	"reflect"
	"testing"
)

// dimensionFilterSQL is the pure, testable unit backing the optional dimension
// filter on cmdFetchMapMarkers/cmdFetchBaseMarkers. It must:
//   - return no clause and no args when dimension is nil (all dimensions —
//     preserves pre-#274 behaviour for callers that omit the param)
//   - return a clause bound to $2 (the map key always occupies $1) and args
//     containing the dimension when set, including the zero value: dimension 0
//     is a real, distinct selection, not "unset"
//   - use the caller-supplied table alias so it composes into both the
//     player/vehicle query (alias "a") and the base query (alias "t")
//   - wrap the column in COALESCE(..., 0) rather than compare it bare, so a
//     NULL dimension_index is treated as bucket 0 — see
//     TestDimensionFilterSQL_NullRowsMatchDimensionZero below for why a bare
//     `= $2` would be a bug.
func TestDimensionFilterSQL(t *testing.T) {
	t.Parallel()

	zero := 0
	three := 3

	tests := []struct {
		name       string
		alias      string
		dimension  *int
		wantClause string
		wantArgs   []any
	}{
		{
			name:       "nil dimension means all dimensions",
			alias:      "a",
			dimension:  nil,
			wantClause: "",
			wantArgs:   nil,
		},
		{
			name:       "dimension zero is a real filter, not absent",
			alias:      "a",
			dimension:  &zero,
			wantClause: " AND COALESCE(a.dimension_index, 0) = $2",
			wantArgs:   []any{0},
		},
		{
			name:       "non-zero dimension",
			alias:      "a",
			dimension:  &three,
			wantClause: " AND COALESCE(a.dimension_index, 0) = $2",
			wantArgs:   []any{3},
		},
		{
			name:       "alias is respected for the base-markers query",
			alias:      "t",
			dimension:  &three,
			wantClause: " AND COALESCE(t.dimension_index, 0) = $2",
			wantArgs:   []any{3},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			clause, args := dimensionFilterSQL(tt.alias, tt.dimension)
			if clause != tt.wantClause {
				t.Errorf("clause = %q, want %q", clause, tt.wantClause)
			}
			if !reflect.DeepEqual(args, tt.wantArgs) {
				t.Errorf("args = %#v, want %#v", args, tt.wantArgs)
			}
		})
	}
}

// TestDimensionFilterSQL_NullRowsMatchDimensionZero locks in the fix for the
// bug found in review: a NULL dimension_index is displayed to the client as
// dimension 0 (mapMarker's own COALESCE(a.dimension_index, 0)) and would never
// appear as its own option in cmdFetchMapDimensions (which also now groups on
// COALESCE(dimension_index, 0)), so the filter MUST agree and also treat NULL
// as bucket 0 — otherwise a NULL-dimension actor is shown as "dimension 0" but
// vanishes the moment the operator actually selects dimension 0, only
// reappearing under "all dimensions".
//
// This package has no pgx mock, so rather than asserting the produced clause
// text only, this executes the actual clause dimensionFilterSQL returns
// against a real database (SQLite in-memory, via the modernc.org/sqlite
// driver already registered by store.go) with a genuine NULL dimension_index
// row, proving the fix end-to-end rather than by construction:
//   - dimension=0 must return both the true-zero row AND the NULL row
//   - dimension=1 must return only the dimension-1 row, not the NULL row
//   - no filter (nil dimension) must return every row, NULL included
//
// SQLite accepts the same "$N" placeholder syntax used against Postgres and
// evaluates COALESCE identically, so the clause under test is exercised
// unmodified — only the surrounding query (a throwaway local table) differs
// from the real dune.actors query.
func TestDimensionFilterSQL_NullRowsMatchDimensionZero(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.Exec(`CREATE TABLE actors (id INTEGER, dimension_index INTEGER)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO actors (id, dimension_index) VALUES
			(1, 0),    -- true dimension 0
			(2, NULL), -- legacy/edge-case row with no dimension recorded
			(3, 1)     -- a different, concrete dimension
	`); err != nil {
		t.Fatalf("seed rows: %v", err)
	}

	queryIDs := func(dimension *int) []int {
		t.Helper()
		clause, args := dimensionFilterSQL("actors", dimension)
		rows, err := db.Query(`SELECT id FROM actors WHERE 1 = 1`+clause, append([]any{0}, args...)...)
		if err != nil {
			t.Fatalf("query (dimension=%v): %v", dimension, err)
		}
		defer func() { _ = rows.Close() }()
		var ids []int
		for rows.Next() {
			var id int
			if err := rows.Scan(&id); err != nil {
				t.Fatalf("scan: %v", err)
			}
			ids = append(ids, id)
		}
		return ids
	}

	zero := 0
	one := 1

	if got := queryIDs(&zero); !reflect.DeepEqual(got, []int{1, 2}) {
		t.Errorf("dimension=0: got ids %v, want [1 2] (the NULL row must be treated as dimension 0)", got)
	}
	if got := queryIDs(&one); !reflect.DeepEqual(got, []int{3}) {
		t.Errorf("dimension=1: got ids %v, want [3] (the NULL row must NOT match a concrete non-zero dimension)", got)
	}
	if got := queryIDs(nil); !reflect.DeepEqual(got, []int{1, 2, 3}) {
		t.Errorf("dimension=nil (all dimensions): got ids %v, want [1 2 3]", got)
	}
}
