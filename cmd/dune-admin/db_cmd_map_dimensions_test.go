package main

import (
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
			wantClause: " AND a.dimension_index = $2",
			wantArgs:   []any{0},
		},
		{
			name:       "non-zero dimension",
			alias:      "a",
			dimension:  &three,
			wantClause: " AND a.dimension_index = $2",
			wantArgs:   []any{3},
		},
		{
			name:       "alias is respected for the base-markers query",
			alias:      "t",
			dimension:  &three,
			wantClause: " AND t.dimension_index = $2",
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
