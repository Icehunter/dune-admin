package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDiscoveredWebInterfaces_NilControlReturnsNil(t *testing.T) {
	t.Parallel()
	if got := discoveredWebInterfaces(context.Background(), nil, &localExecutor{}); got != nil {
		t.Errorf("nil control: got %v, want nil", got)
	}
}

func TestDiscoveredWebInterfaces_NilExecutorReturnsNil(t *testing.T) {
	t.Parallel()
	ctrl := &statusFakeControl{}
	if got := discoveredWebInterfaces(context.Background(), ctrl, nil); got != nil {
		t.Errorf("nil executor: got %v, want nil", got)
	}
}

// withoutDiscoveredDirector drops the control-plane-discovered "Battlegroup
// Director" entry when director_url is configured, so the card doesn't render the
// Director twice (once as the automatic DirectorRow, once from discovery).
// (Icehunter review point 4.)
func TestWithoutDiscoveredDirector(t *testing.T) {
	t.Parallel()
	director := webInterface{Label: directorInterfaceLabel, URL: "http://host:31003/"}
	fb := webInterface{Label: "File Browser", URL: "http://host:18888/"}

	labels := func(ws []webInterface) []string {
		out := make([]string, len(ws))
		for i, w := range ws {
			out[i] = w.Label
		}
		return out
	}

	tests := []struct {
		name        string
		in          []webInterface
		directorURL string
		want        []string
	}{
		{"director_url set → drop discovered director", []webInterface{director, fb}, "http://127.0.0.1:11717", []string{"File Browser"}},
		{"director_url empty → keep all", []webInterface{director, fb}, "", []string{directorInterfaceLabel, "File Browser"}},
		{"no director in list → unchanged", []webInterface{fb}, "http://x", []string{"File Browser"}},
		{"nil input → empty", nil, "http://x", []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := labels(withoutDiscoveredDirector(tt.in, tt.directorURL))
			if len(got) != len(tt.want) {
				t.Fatalf("labels = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("labels = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestHandleGetWebInterfaces_NilControlNoDiscovered(t *testing.T) {
	orig := globalControl
	origExec := globalExecutor
	globalControl = nil
	globalExecutor = nil
	t.Cleanup(func() {
		globalControl = orig
		globalExecutor = origExec
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/web-interfaces", nil)
	rr := httptest.NewRecorder()
	handleGetWebInterfaces(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}
