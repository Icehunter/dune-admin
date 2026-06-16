package main

import (
	"strings"
	"testing"
)

const ampDefaultsSuffix = "/extracted/game-server/home/dune/server/DuneSandbox/Config"

// TestAmpDefaultINIDir verifies the AMP stock-defaults directory is derived from
// the instance layout: from the discovered INI dir, the configured server_ini_dir,
// or the conventional ampdata path for the instance — in that order.
func TestAmpDefaultINIDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		ctrl   *ampControl
		iniDir string
		want   string
	}{
		{
			name:   "from discovered ue5-saved dir",
			ctrl:   &ampControl{},
			iniDir: "/home/amp/.ampdata/instances/DuneAwakening01/duneawakening/server/state/ue5-saved/UserSettings",
			want:   "/home/amp/.ampdata/instances/DuneAwakening01/duneawakening" + ampDefaultsSuffix,
		},
		{
			name:   "from configured server_ini_dir when no discovered dir",
			ctrl:   &ampControl{iniDir: "/opt/inst/duneawakening/server/state"},
			iniDir: "",
			want:   "/opt/inst/duneawakening" + ampDefaultsSuffix,
		},
		{
			name:   "from instance when nothing else available (container)",
			ctrl:   &ampControl{useContainer: true, instance: "DA02", ampUser: "amp"},
			iniDir: "",
			want:   "/home/amp/.ampdata/instances/DA02/duneawakening" + ampDefaultsSuffix,
		},
		{
			name:   "empty when nothing derivable",
			ctrl:   &ampControl{},
			iniDir: "/some/unrelated/path",
			want:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ctrl.defaultINIDir(tt.iniDir); got != tt.want {
				t.Errorf("defaultINIDir(%q) = %q, want %q", tt.iniDir, got, tt.want)
			}
		})
	}
}

// TestDiscoverViaControlDefaultDir verifies the discovery glue reads the stock
// default from the control-plane-derived directory, and is a no-op when the
// control plane does not provide one.
func TestDiscoverViaControlDefaultDir(t *testing.T) {
	origCtrl, origExec := globalControl, globalExecutor
	t.Cleanup(func() { globalControl, globalExecutor = origCtrl, origExec })

	wantDir := "/home/amp/.ampdata/instances/DuneAwakening01/duneawakening" + ampDefaultsSuffix
	globalControl = &ampControl{useContainer: true, instance: "DuneAwakening01", ampUser: "amp"}
	globalExecutor = &fnExecutor{fn: func(cmd string) (string, error) {
		if strings.Contains(cmd, wantDir+"/DefaultGame.ini") {
			return "[Sec]\nKey=val\n", nil
		}
		return "", nil
	}}

	if got := discoverViaControlDefaultDir("", "DefaultGame.ini", globalControl, globalExecutor); !strings.Contains(got, "Key=val") {
		t.Errorf("expected content from %s, got %q", wantDir, got)
	}

	// Non-provider control plane → no derivation.
	globalControl = &localControl{}
	if got := discoverViaControlDefaultDir("", "DefaultGame.ini", globalControl, globalExecutor); got != "" {
		t.Errorf("expected empty for non-AMP control, got %q", got)
	}
}
