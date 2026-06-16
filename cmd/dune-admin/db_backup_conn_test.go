package main

import "testing"

// TestDBBackupConn_UsesActiveServerConfig guards the regression where the manual
// DB-backup path read the global loadedConfig flat fields (cleared by the storage
// remodel) and dumped the wrong DB — e.g. the :5432 default instead of the AMP
// server's :15432. The connection must come from the active server's ServerConfig.
func TestDBBackupConn_UsesActiveServerConfig(t *testing.T) {
	orig := globalRegistry
	t.Cleanup(func() { globalRegistry = orig })

	globalRegistry = newServerRegistry(nil)
	globalRegistry.Register(&ServerContext{
		ID:   "1",
		Name: "AMP",
		Cfg:  ServerConfig{ID: 1, DBPort: 15432, DBName: "dune", DBUser: "dune", DBPass: "secret"},
	})
	if err := globalRegistry.SetActive("1"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}

	c := dbBackupConn()
	if c.Port != 15432 {
		t.Errorf("dbBackupConn().Port = %d, want 15432 (active server) — not the :5432 default", c.Port)
	}
	if c.Pass != "secret" || c.Name != "dune" {
		t.Errorf("dbBackupConn() not from active server: %+v", c)
	}
}
