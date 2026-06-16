package main

import (
	"os"
	"testing"
)

func TestEffectiveListenAddr(t *testing.T) {
	tests := []struct {
		name     string
		flagAddr string
		explicit bool
		dbAddr   string
		want     string
	}{
		{
			name:     "explicit flag overrides DB value",
			flagAddr: ":9090",
			explicit: true,
			dbAddr:   ":18080",
			want:     ":9090",
		},
		{
			name:     "explicit flag overrides empty DB",
			flagAddr: ":9090",
			explicit: true,
			dbAddr:   "",
			want:     ":9090",
		},
		{
			name:     "DB value wins when not explicit",
			flagAddr: ":8080",
			explicit: false,
			dbAddr:   ":18080",
			want:     ":18080",
		},
		{
			name:     "falls back to flag default when not explicit and DB empty",
			flagAddr: ":8080",
			explicit: false,
			dbAddr:   "",
			want:     ":8080",
		},
		{
			name:     "DB value wins even when flag is set to default",
			flagAddr: ":8080",
			explicit: false,
			dbAddr:   ":1234",
			want:     ":1234",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := effectiveListenAddr(tt.flagAddr, tt.explicit, tt.dbAddr)
			if got != tt.want {
				t.Errorf("effectiveListenAddr(%q, %v, %q) = %q; want %q",
					tt.flagAddr, tt.explicit, tt.dbAddr, got, tt.want)
			}
		})
	}
}

// TestResolveListenAddr is the regression test for the crash-loop:
// with config.yaml absent, the DB value (:18080) must be used — not the :8080 default.
func TestResolveListenAddr(t *testing.T) {
	// Save and restore global state.
	origListenAddr := listenAddr
	origAddrFlagSet := addrFlagSet
	origLoadedConfig := loadedConfig
	t.Cleanup(func() {
		listenAddr = origListenAddr
		addrFlagSet = origAddrFlagSet
		loadedConfig = origLoadedConfig
	})

	t.Run("DB value wins when no flag or LISTEN_ADDR env set", func(t *testing.T) {
		if err := os.Unsetenv("LISTEN_ADDR"); err != nil {
			t.Fatalf("unsetenv LISTEN_ADDR: %v", err)
		}
		listenAddr = ":8080"               // what init() produces with no env seed
		addrFlagSet = false                // -addr was not passed
		loadedConfig.ListenAddr = ":18080" // saved in DB, hydrated on boot
		got := resolveListenAddr()
		if got != ":18080" {
			t.Errorf("got %q; want :18080 — DB value must win when config.yaml is absent", got)
		}
	})

	t.Run("explicit -addr flag overrides DB value", func(t *testing.T) {
		if err := os.Unsetenv("LISTEN_ADDR"); err != nil {
			t.Fatalf("unsetenv LISTEN_ADDR: %v", err)
		}
		listenAddr = ":9090"
		addrFlagSet = true
		loadedConfig.ListenAddr = ":18080"
		got := resolveListenAddr()
		if got != ":9090" {
			t.Errorf("got %q; want :9090 — -addr flag must win over DB", got)
		}
	})

	t.Run("LISTEN_ADDR env overrides DB value", func(t *testing.T) {
		t.Setenv("LISTEN_ADDR", ":7070")
		listenAddr = ":7070" // flag picked it up from env
		addrFlagSet = false  // but -addr was not explicitly passed
		loadedConfig.ListenAddr = ":18080"
		got := resolveListenAddr()
		if got != ":7070" {
			t.Errorf("got %q; want :7070 — LISTEN_ADDR env must win over DB", got)
		}
	})

	t.Run("falls back to default when DB empty and no flag or env", func(t *testing.T) {
		if err := os.Unsetenv("LISTEN_ADDR"); err != nil {
			t.Fatalf("unsetenv LISTEN_ADDR: %v", err)
		}
		listenAddr = ":8080"
		addrFlagSet = false
		loadedConfig.ListenAddr = ""
		got := resolveListenAddr()
		if got != ":8080" {
			t.Errorf("got %q; want :8080 — built-in default must be used when DB empty", got)
		}
	})
}
