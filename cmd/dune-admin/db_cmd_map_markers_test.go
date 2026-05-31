package main

import "testing"

// Phase 1 (Live Map): the map-key allow-list is the pure, testable unit of the
// markers query. v1 scope is open-world only (Hagga Basin + Deep Desert); the
// city instances (Arrakeen/HarkoVillage) are deliberately out of scope. The key
// is also the one piece of caller-supplied input that reaches the query, so the
// allow-list doubles as an injection guard even though the query parameterises it.
func TestValidateMapKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{name: "hagga basin", key: "HaggaBasin", wantErr: false},
		{name: "deep desert", key: "DeepDesert", wantErr: false},
		{name: "city out of v1 scope", key: "Arrakeen", wantErr: true},
		{name: "unknown map", key: "Atlantis", wantErr: true},
		{name: "empty", key: "", wantErr: true},
		{name: "injection attempt", key: "HaggaBasin'; DROP TABLE dune.actors; --", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateMapKey(tt.key)
			if tt.wantErr && err == nil {
				t.Fatalf("validateMapKey(%q): expected error, got nil", tt.key)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("validateMapKey(%q): unexpected error: %v", tt.key, err)
			}
		})
	}
}
