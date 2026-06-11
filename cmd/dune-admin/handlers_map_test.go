package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// handleGetMapMarkers validates the ?map= input before touching the DB, so bad
// input fails fast with 400 and a valid map with no DB connection surfaces 503.
// globalDB is nil in unit tests (connectAll is never called), which lets us
// exercise the input + guard paths without a database. Not parallel: it reads
// the globalDB package global.
func TestHandleListMaps_NilDB(t *testing.T) {
	orig := globalDB
	globalDB = nil
	defer func() { globalDB = orig }()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/maps", nil)
	rec := httptest.NewRecorder()
	handleListMaps(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", rec.Code)
	}
}

func TestHandleGetMapMarkers_Input(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantStatus int
	}{
		{name: "missing map param", query: "", wantStatus: http.StatusBadRequest},
		{name: "unsupported map", query: "?map=Atlantis", wantStatus: http.StatusBadRequest},
		{name: "valid map, db down", query: "?map=HaggaBasin", wantStatus: http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/map/markers"+tt.query, nil)
			rec := httptest.NewRecorder()
			handleGetMapMarkers(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}
