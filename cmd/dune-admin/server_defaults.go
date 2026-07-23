package main

// Per-control-plane connection defaults.
//
// A fresh web setup mimics the old console wizard: leaving a field blank uses a
// sensible default rather than an empty value. The web form submits empty
// strings (and 0 for ports), so these helpers gap-fill before the config is
// connected and persisted. Defaults match the CLI flag defaults in main.go.

const (
	defaultDBHost   = "127.0.0.1"
	defaultDBPort   = 15432
	defaultDBUser   = "dune"
	defaultDBName   = "dune"
	defaultDBSchema = "dune"

	// AMP defaults mirror the console setup wizard so a minimal AMP server fills
	// in the standard CubeCoders layout.
	// defaultAmpAPIPort is defined in amp_api.go (8081).
	// defaultAmpAPIHost is defined in amp_api.go (127.0.0.1).
	defaultAmpInstance = "DuneAwakening01"
	defaultAmpLogPath  = "/AMP/duneawakening/logs"
	defaultAmpAPIUser  = "admin"
	defaultAmpAPIPass  = "admin"
)

// defaultSSHUserFor returns the default SSH user for a control plane. AMP runs
// as the "amp" OS user; everything else defaults to "dune".
func defaultSSHUserFor(control string) string {
	if control == "amp" {
		return "amp"
	}
	return "dune"
}

// usesSSH reports whether a control plane connects over SSH (and therefore needs
// an SSH key auto-filled). kubectl and amp tunnel through SSH; local/docker only
// do when an explicit ssh_host is set.
func usesSSH(control, sshHost string) bool {
	return control == "kubectl" || control == "amp" || sshHost != ""
}

// applyServerConfigDefaults gap-fills empty per-server connection fields with
// control-plane-appropriate defaults. Idempotent: explicit values are preserved.
func applyServerConfigDefaults(cfg *ServerConfig) {
	ctrl := cfg.Control
	if ctrl == "" {
		ctrl = "local"
	}
	if cfg.DBHost == "" {
		cfg.DBHost = defaultDBHost
	}
	if cfg.DBPort == 0 {
		cfg.DBPort = defaultDBPort
	}
	if cfg.DBUser == "" {
		cfg.DBUser = defaultDBUser
	}
	if cfg.DBName == "" {
		cfg.DBName = defaultDBName
	}
	if cfg.DBSchema == "" {
		cfg.DBSchema = defaultDBSchema
	}
	if cfg.SSHUser == "" {
		cfg.SSHUser = defaultSSHUserFor(ctrl)
	}
	if cfg.SSHKey == "" && usesSSH(ctrl, cfg.SSHHost) {
		cfg.SSHKey = discoverSSHKeyPath()
	}
	if ctrl == "amp" {
		applyAmpServerDefaults(cfg)
	}
}

// applyAmpServerDefaults fills the standard CubeCoders AMP layout for blank
// fields. The container name tracks the (possibly defaulted) instance name.
func applyAmpServerDefaults(cfg *ServerConfig) {
	if cfg.AmpUser == "" {
		cfg.AmpUser = "amp"
	}
	if cfg.AmpInstance == "" {
		cfg.AmpInstance = defaultAmpInstance
	}
	if cfg.AmpContainer == "" {
		cfg.AmpContainer = "AMP_" + cfg.AmpInstance
	}
	if cfg.AmpLogPath == "" {
		cfg.AmpLogPath = defaultAmpLogPath
	}
	if cfg.AmpAPIUser == "" {
		cfg.AmpAPIUser = defaultAmpAPIUser
	}
	if cfg.AmpAPIPass == "" {
		cfg.AmpAPIPass = defaultAmpAPIPass
	}
	if cfg.AmpAPIHost == "" {
		cfg.AmpAPIHost = defaultAmpAPIHost
	}
	if cfg.AmpAPIPort == 0 {
		cfg.AmpAPIPort = defaultAmpAPIPort
	}
	if cfg.AmpUseContainer == nil {
		t := true
		cfg.AmpUseContainer = &t
	}
	if cfg.DefaultIniDir == "" {
		cfg.DefaultIniDir = ampDefaultIniDir(cfg.AmpInstance)
	}
}

// ampDefaultIniDir returns the standard AMP extracted game-server Config path for
// an instance — where DefaultGame.ini lives under a CubeCoders AMP install.
func ampDefaultIniDir(instance string) string {
	return "/home/amp/.ampdata/instances/" + instance +
		"/duneawakening/extracted/game-server/home/dune/server/DuneSandbox/Config"
}

// applyFlatConnectionDefaults is the appConfig (legacy/default-server) analogue
// of applyServerConfigDefaults. Used by the fresh-setup flat-config save path so
// blank fields don't wipe the flag defaults via applyConfig.
func applyFlatConnectionDefaults(cfg *appConfig) {
	ctrl := cfg.Control
	if ctrl == "" {
		ctrl = "local"
	}
	if cfg.DBHost == "" {
		cfg.DBHost = defaultDBHost
	}
	if cfg.DBPort == 0 {
		cfg.DBPort = defaultDBPort
	}
	if cfg.DBUser == "" {
		cfg.DBUser = defaultDBUser
	}
	if cfg.DBName == "" {
		cfg.DBName = defaultDBName
	}
	if cfg.DBSchema == "" {
		cfg.DBSchema = defaultDBSchema
	}
	if cfg.SSHUser == "" {
		cfg.SSHUser = defaultSSHUserFor(ctrl)
	}
	if cfg.SSHKey == "" && usesSSH(ctrl, cfg.SSHHost) {
		cfg.SSHKey = discoverSSHKeyPath()
	}
	if ctrl == "amp" {
		applyAmpFlatDefaults(cfg)
	}
}

// applyAmpFlatDefaults is the appConfig analogue of applyAmpServerDefaults.
func applyAmpFlatDefaults(cfg *appConfig) {
	if cfg.AmpUser == "" {
		cfg.AmpUser = "amp"
	}
	if cfg.AmpInstance == "" {
		cfg.AmpInstance = defaultAmpInstance
	}
	if cfg.AmpContainer == "" {
		cfg.AmpContainer = "AMP_" + cfg.AmpInstance
	}
	if cfg.AmpLogPath == "" {
		cfg.AmpLogPath = defaultAmpLogPath
	}
	if cfg.AmpAPIUser == "" {
		cfg.AmpAPIUser = defaultAmpAPIUser
	}
	if cfg.AmpAPIPass == "" {
		cfg.AmpAPIPass = defaultAmpAPIPass
	}
	if cfg.AmpAPIHost == "" {
		cfg.AmpAPIHost = defaultAmpAPIHost
	}
	if cfg.AmpAPIPort == 0 {
		cfg.AmpAPIPort = defaultAmpAPIPort
	}
	if cfg.AmpUseContainer == nil {
		t := true
		cfg.AmpUseContainer = &t
	}
	if cfg.DefaultIniDir == "" {
		cfg.DefaultIniDir = ampDefaultIniDir(cfg.AmpInstance)
	}
}
