package main

import "testing"

// TestWebInterfacesFromAddresses covers the kubectl-discovered director/file
// browser links (#director-files-from-kubectl). The CRD advertises a node IP the
// operator may not be able to route to, so the host is rewritten to the VM IP
// dune-admin connects to (vmHost) while keeping the node port. Empty addresses
// are skipped.
func TestWebInterfacesFromAddresses(t *testing.T) {
	tests := []struct {
		name     string
		vmHost   string
		director string
		files    string
		want     []webInterface
	}{
		{
			name:     "rewrites the CRD node IP to the VM IP, keeps the port",
			vmHost:   "192.168.0.67",
			director: "207.216.171.194:31592",
			files:    "207.216.171.194:18888",
			want: []webInterface{
				{Label: "Battlegroup Director", URL: "http://192.168.0.67:31592/", Target: "207.216.171.194:31592"},
				{Label: "File Browser", URL: "http://192.168.0.67:18888/", Target: "207.216.171.194:18888"},
			},
		},
		{
			name:     "only file browser",
			vmHost:   "192.168.0.67",
			director: "",
			files:    "207.216.171.194:18888",
			want:     []webInterface{{Label: "File Browser", URL: "http://192.168.0.67:18888/", Target: "207.216.171.194:18888"}},
		},
		{
			name:     "no vmHost (local executor) falls back to the reported host",
			vmHost:   "",
			director: "207.216.171.194:31592",
			files:    "",
			want:     []webInterface{{Label: "Battlegroup Director", URL: "http://207.216.171.194:31592/", Target: "207.216.171.194:31592"}},
		},
		{
			name:     "neither (none discovered)",
			vmHost:   "192.168.0.67",
			director: "  ",
			files:    "",
			want:     nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := webInterfacesFromAddresses(tt.vmHost, tt.director, tt.files)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d interfaces, want %d: %+v", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("[%d] got %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
