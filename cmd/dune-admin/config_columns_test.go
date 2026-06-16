package main

import (
	"database/sql"
	"testing"
)

func TestBoolPtrCodec(t *testing.T) {
	tru, fls := true, false
	cases := []struct {
		name string
		in   *bool
	}{{"nil", nil}, {"true", &tru}, {"false", &fls}}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := nullIntToBoolPtr(boolPtrToNullInt(c.in))
			if (got == nil) != (c.in == nil) {
				t.Fatalf("nil-ness changed: in=%v got=%v", c.in, got)
			}
			if got != nil && *got != *c.in {
				t.Fatalf("value changed: in=%v got=%v", *c.in, *got)
			}
		})
	}
}

func TestRunColumnMigrationOnce(t *testing.T) {
	db := openSharedScopeDB(t)
	calls := 0
	fn := func(*sql.Tx) error { calls++; return nil }
	for i := range 3 {
		if err := runColumnMigrationOnce(db, "migrated:test_columns", fn); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	if calls != 1 {
		t.Fatalf("migration ran %d times, want 1", calls)
	}
}
