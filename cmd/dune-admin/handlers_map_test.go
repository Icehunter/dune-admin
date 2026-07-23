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
		// #274: dimension is optional and validated before the DB guard, so a bad
		// value 400s even with no DB connected — map validation and dimension
		// parsing are both pure input checks that must run first.
		{name: "non-numeric dimension", query: "?map=HaggaBasin&dimension=abc", wantStatus: http.StatusBadRequest},
		{name: "negative dimension", query: "?map=HaggaBasin&dimension=-1", wantStatus: http.StatusBadRequest},
		{name: "valid map + valid dimension, db down", query: "?map=HaggaBasin&dimension=0", wantStatus: http.StatusServiceUnavailable},
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

// #274: parseDimensionParam is the pure unit deciding what caller input reaches
// the query. Absence must mean "all dimensions" (nil, no error) to preserve
// pre-#274 behaviour for any existing caller that omits the param.
func TestParseDimensionParam(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		query   string
		want    *int
		wantErr bool
	}{
		{name: "absent means all dimensions", query: "", want: nil},
		{name: "empty string means all dimensions", query: "?dimension=", want: nil},
		{name: "zero is a real dimension", query: "?dimension=0", want: intPtr(0)},
		{name: "positive dimension", query: "?dimension=3", want: intPtr(3)},
		{name: "negative rejected", query: "?dimension=-1", wantErr: true},
		{name: "non-numeric rejected", query: "?dimension=abc", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/map/markers"+tt.query, nil)
			got, err := parseDimensionParam(req)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (value %v)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if (got == nil) != (tt.want == nil) {
				t.Fatalf("got = %v, want %v", got, tt.want)
			}
			if got != nil && *got != *tt.want {
				t.Fatalf("got = %d, want %d", *got, *tt.want)
			}
		})
	}
}

// #274: /api/v1/map/dimensions lists the available dimensions for a map so the
// frontend can populate a selector. Input validation mirrors /map/markers.
func TestHandleGetMapDimensions_Input(t *testing.T) {
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
			req := httptest.NewRequest(http.MethodGet, "/api/v1/map/dimensions"+tt.query, nil)
			rec := httptest.NewRecorder()
			handleGetMapDimensions(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}
