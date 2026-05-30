package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderK8SManifest(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "manifest.yaml")
	if err := renderK8SManifest(out); err != nil {
		t.Fatalf("renderK8SManifest: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	manifest := string(data)

	checks := []struct {
		desc    string
		want    string
		present bool
	}{
		// PVCs
		{"dune-admin-app PVC exists", "name: dune-admin-app", true},
		{"market-bot-cache PVC exists", "name: market-bot-cache", true},

		// Init containers seed binary and config
		{"seed-binary init container exists", "name: seed-binary", true},
		{"seed-config init container exists", "name: seed-config", true},
		{"seed-binary copies from seed dir", "/usr/local/share/dune-admin-seed/dune-admin", true},
		{"seed-binary uses build-time stamp", ".image-build-time", true},
		{"seed-config copies config.yaml", "cp /configmap/config.yaml /app-config/config.yaml", true},

		// Writable mounts
		{"app-rw PVC mounted at /app", "mountPath: /app", true},
		{"config-rw emptyDir mounted at /root/.dune-admin", "mountPath: /root/.dune-admin", true},
		{"config-rw volume is emptyDir", "emptyDir: {}", true},

		// No read-only ConfigMap subPath mount for config
		{"no subPath config mount", "subPath: config.yaml", false},
	}

	for _, c := range checks {
		t.Run(c.desc, func(t *testing.T) {
			found := strings.Contains(manifest, c.want)
			if found != c.present {
				if c.present {
					t.Errorf("manifest missing %q", c.want)
				} else {
					t.Errorf("manifest should not contain %q", c.want)
				}
			}
		})
	}
}
