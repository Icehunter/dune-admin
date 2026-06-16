package main

import "testing"

// TestAmpGameOverridePath verifies the AMP override path derivation: game
// settings live in UserOverrides.ini in the state dir, two levels up from the
// discovered ue5-saved/UserSettings INI dir.
func TestAmpGameOverridePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		dir  string
		want string
	}{
		{
			name: "ue5-saved layout",
			dir:  "/home/amp/.ampdata/instances/DuneAwakening01/duneawakening/server/state/ue5-saved/UserSettings",
			want: "/home/amp/.ampdata/instances/DuneAwakening01/duneawakening/server/state/UserOverrides.ini",
		},
		{
			name: "trailing slash",
			dir:  "/srv/state/ue5-saved/UserSettings/",
			want: "/srv/state/UserOverrides.ini",
		},
		{
			name: "non-ue5 dir falls back to sibling",
			dir:  "/srv/state",
			want: "/srv/state/UserOverrides.ini",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := (&ampControl{}).gameOverridePath(tt.dir)
			if got != tt.want {
				t.Errorf("gameOverridePath(%q) = %q, want %q", tt.dir, got, tt.want)
			}
		})
	}
}

// TestGameWritePath_AMPUsesOverrides verifies game-scoped writes go to
// UserOverrides.ini when the control plane is AMP (a gameOverrideProvider).
func TestGameWritePath_AMPUsesOverrides(t *testing.T) {
	orig := globalControl
	globalControl = &ampControl{}
	t.Cleanup(func() { globalControl = orig })

	dir := "/srv/state/ue5-saved/UserSettings"
	got := gameWritePath(dir, globalControl)
	want := "/srv/state/UserOverrides.ini"
	if got != want {
		t.Errorf("gameWritePath = %q, want %q", got, want)
	}
}

// TestGameWritePath_NonAMPWritesUserGame verifies non-AMP control planes keep
// writing game settings directly to UserGame.ini in dir.
func TestGameWritePath_NonAMPWritesUserGame(t *testing.T) {
	orig := globalControl
	t.Cleanup(func() { globalControl = orig })

	dir := "/k8s/config"
	for _, ctrl := range []ControlPlane{&localControl{}, &kubectlControl{}, nil} {
		globalControl = ctrl
		got := gameWritePath(dir, globalControl)
		want := dir + "/UserGame.ini"
		if got != want {
			t.Errorf("control %T: gameWritePath = %q, want %q", ctrl, got, want)
		}
	}
}

// TestBuildLayerSources_OverridesWin verifies the userGameOverrides layer takes
// precedence over userGame (AMP-managed) for the same key, since AMP appends
// UserOverrides.ini after UserGame.ini at boot.
func TestBuildLayerSources_OverridesWin(t *testing.T) {
	t.Parallel()

	gameIni := map[string]map[string]string{secBuilding: {"m_MaxLandclaim": "100"}}
	overridesIni := map[string]map[string]string{secBuilding: {"m_MaxLandclaim": "250"}}

	layers := buildLayerSources(nil, nil, nil, gameIni, overridesIni)

	s := &ServerSetting{Section: secBuilding, Key: "m_MaxLandclaim", Type: string(settingInt)}
	applySettingLayers(s, layers)

	if s.Current != "250" {
		t.Errorf("Current = %q, want 250 (override should win)", s.Current)
	}
	if s.Source != "userGameOverrides" {
		t.Errorf("Source = %q, want userGameOverrides", s.Source)
	}
	if !s.IsOverride {
		t.Error("IsOverride = false, want true")
	}
	if len(s.Layers) != 2 {
		t.Fatalf("got %d layers, want 2 (userGame + userGameOverrides)", len(s.Layers))
	}
	if s.Layers[0].Source != "userGame" || s.Layers[1].Source != "userGameOverrides" {
		t.Errorf("layer order = [%s,%s], want [userGame,userGameOverrides]", s.Layers[0].Source, s.Layers[1].Source)
	}
}
