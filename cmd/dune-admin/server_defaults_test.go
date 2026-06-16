package main

import "testing"

func TestApplyServerConfigDefaults_DBFields(t *testing.T) {
	cfg := ServerConfig{Control: "local"}
	applyServerConfigDefaults(&cfg)

	if cfg.DBHost != "127.0.0.1" {
		t.Errorf("DBHost = %q, want 127.0.0.1", cfg.DBHost)
	}
	if cfg.DBPort != 15432 {
		t.Errorf("DBPort = %d, want 15432", cfg.DBPort)
	}
	if cfg.DBUser != "dune" {
		t.Errorf("DBUser = %q, want dune", cfg.DBUser)
	}
	if cfg.DBName != "dune" {
		t.Errorf("DBName = %q, want dune", cfg.DBName)
	}
	if cfg.DBSchema != "dune" {
		t.Errorf("DBSchema = %q, want dune", cfg.DBSchema)
	}
}

func TestApplyServerConfigDefaults_PreservesExplicit(t *testing.T) {
	cfg := ServerConfig{
		Control: "local", DBHost: "10.0.0.5", DBPort: 5432, DBUser: "pg",
		DBName: "game", DBSchema: "public", SSHUser: "ubuntu",
	}
	applyServerConfigDefaults(&cfg)

	if cfg.DBHost != "10.0.0.5" || cfg.DBPort != 5432 || cfg.DBUser != "pg" ||
		cfg.DBName != "game" || cfg.DBSchema != "public" || cfg.SSHUser != "ubuntu" {
		t.Errorf("explicit values must be preserved, got %+v", cfg)
	}
}

func TestApplyServerConfigDefaults_SSHUserPerControlPlane(t *testing.T) {
	tests := []struct {
		control string
		want    string
	}{
		{"amp", "amp"},
		{"kubectl", "dune"},
		{"local", "dune"},
		{"docker", "dune"},
		{"", "dune"},
	}
	for _, tt := range tests {
		t.Run(tt.control, func(t *testing.T) {
			cfg := ServerConfig{Control: tt.control}
			applyServerConfigDefaults(&cfg)
			if cfg.SSHUser != tt.want {
				t.Errorf("control %q: SSHUser = %q, want %q", tt.control, cfg.SSHUser, tt.want)
			}
		})
	}
}

func TestApplyServerConfigDefaults_AmpUser(t *testing.T) {
	cfg := ServerConfig{Control: "amp"}
	applyServerConfigDefaults(&cfg)
	if cfg.AmpUser != "amp" {
		t.Errorf("AmpUser = %q, want amp", cfg.AmpUser)
	}
}

func TestApplyServerConfigDefaults_AmpFields(t *testing.T) {
	cfg := ServerConfig{Control: "amp"}
	applyServerConfigDefaults(&cfg)

	if cfg.AmpInstance != "DuneAwakening01" {
		t.Errorf("AmpInstance = %q, want DuneAwakening01", cfg.AmpInstance)
	}
	if cfg.AmpContainer != "AMP_DuneAwakening01" {
		t.Errorf("AmpContainer = %q, want AMP_DuneAwakening01", cfg.AmpContainer)
	}
	if cfg.AmpLogPath != "/AMP/duneawakening/logs" {
		t.Errorf("AmpLogPath = %q, want /AMP/duneawakening/logs", cfg.AmpLogPath)
	}
	if cfg.AmpAPIUser != "admin" {
		t.Errorf("AmpAPIUser = %q, want admin", cfg.AmpAPIUser)
	}
	if cfg.AmpAPIPort != 8081 {
		t.Errorf("AmpAPIPort = %d, want 8081", cfg.AmpAPIPort)
	}
	if cfg.AmpUseContainer == nil || !*cfg.AmpUseContainer {
		t.Error("AmpUseContainer should default to true")
	}
}

// Container default tracks a custom instance name.
func TestApplyServerConfigDefaults_AmpContainerTracksInstance(t *testing.T) {
	cfg := ServerConfig{Control: "amp", AmpInstance: "MyDune02"}
	applyServerConfigDefaults(&cfg)
	if cfg.AmpContainer != "AMP_MyDune02" {
		t.Errorf("AmpContainer = %q, want AMP_MyDune02", cfg.AmpContainer)
	}
}

// AMP defaults must not be applied to non-AMP control planes.
func TestApplyServerConfigDefaults_NoAmpFieldsForNonAmp(t *testing.T) {
	cfg := ServerConfig{Control: "local"}
	applyServerConfigDefaults(&cfg)
	if cfg.AmpInstance != "" || cfg.AmpContainer != "" || cfg.AmpAPIPort != 0 {
		t.Errorf("AMP fields should be empty for local control, got %+v", cfg)
	}
}

// Explicit AMP values are preserved (e.g. a non-default API password/port).
func TestApplyServerConfigDefaults_PreservesExplicitAmp(t *testing.T) {
	cfg := ServerConfig{Control: "amp", AmpInstance: "X", AmpAPIUser: "root", AmpAPIPort: 9999}
	applyServerConfigDefaults(&cfg)
	if cfg.AmpAPIUser != "root" || cfg.AmpAPIPort != 9999 {
		t.Errorf("explicit AMP values overwritten: %+v", cfg)
	}
}

func TestApplyServerConfigDefaults_AmpDefaultIniDir(t *testing.T) {
	cfg := ServerConfig{Control: "amp"}
	applyServerConfigDefaults(&cfg)
	want := "/home/amp/.ampdata/instances/DuneAwakening01/duneawakening/extracted/game-server/home/dune/server/DuneSandbox/Config"
	if cfg.DefaultIniDir != want {
		t.Errorf("DefaultIniDir = %q, want %q", cfg.DefaultIniDir, want)
	}
}

func TestApplyServerConfigDefaults_AmpDefaultIniDirTracksInstance(t *testing.T) {
	cfg := ServerConfig{Control: "amp", AmpInstance: "MyDune02"}
	applyServerConfigDefaults(&cfg)
	want := "/home/amp/.ampdata/instances/MyDune02/duneawakening/extracted/game-server/home/dune/server/DuneSandbox/Config"
	if cfg.DefaultIniDir != want {
		t.Errorf("DefaultIniDir = %q, want %q", cfg.DefaultIniDir, want)
	}
}

func TestApplyServerConfigDefaults_PreservesExplicitIniDir(t *testing.T) {
	cfg := ServerConfig{Control: "amp", DefaultIniDir: "/custom/path"}
	applyServerConfigDefaults(&cfg)
	if cfg.DefaultIniDir != "/custom/path" {
		t.Errorf("explicit DefaultIniDir overwritten: %q", cfg.DefaultIniDir)
	}
}

func TestApplyFlatConnectionDefaults_AmpDefaultIniDir(t *testing.T) {
	cfg := appConfig{Control: "amp"}
	applyFlatConnectionDefaults(&cfg)
	want := "/home/amp/.ampdata/instances/DuneAwakening01/duneawakening/extracted/game-server/home/dune/server/DuneSandbox/Config"
	if cfg.DefaultIniDir != want {
		t.Errorf("flat DefaultIniDir = %q, want %q", cfg.DefaultIniDir, want)
	}
}

func TestApplyServerConfigDefaults_SSHKeyForSSHControlPlanes(t *testing.T) {
	// kubectl and amp auto-fill the SSH key when empty; local without an
	// ssh_host leaves it empty (no SSH used).
	kube := ServerConfig{Control: "kubectl"}
	applyServerConfigDefaults(&kube)
	if kube.SSHKey == "" {
		t.Error("kubectl should auto-fill SSHKey when empty")
	}

	local := ServerConfig{Control: "local"}
	applyServerConfigDefaults(&local)
	if local.SSHKey != "" {
		t.Errorf("local without ssh_host should leave SSHKey empty, got %q", local.SSHKey)
	}

	localSSH := ServerConfig{Control: "local", SSHHost: "1.2.3.4:22"}
	applyServerConfigDefaults(&localSSH)
	if localSSH.SSHKey == "" {
		t.Error("local with ssh_host should auto-fill SSHKey")
	}
}

func TestApplyFlatConnectionDefaults_FillsEmpties(t *testing.T) {
	cfg := appConfig{Control: "amp"}
	applyFlatConnectionDefaults(&cfg)

	if cfg.DBHost != "127.0.0.1" || cfg.DBPort != 15432 || cfg.DBUser != "dune" ||
		cfg.DBName != "dune" || cfg.DBSchema != "dune" {
		t.Errorf("flat DB defaults not applied: %+v", cfg)
	}
	if cfg.SSHUser != "amp" {
		t.Errorf("amp flat SSHUser = %q, want amp", cfg.SSHUser)
	}
}

func TestApplyFlatConnectionDefaults_PreservesExplicit(t *testing.T) {
	cfg := appConfig{Control: "local", DBPort: 5432, DBUser: "pg"}
	applyFlatConnectionDefaults(&cfg)
	if cfg.DBPort != 5432 || cfg.DBUser != "pg" {
		t.Errorf("explicit flat values overwritten: %+v", cfg)
	}
}
