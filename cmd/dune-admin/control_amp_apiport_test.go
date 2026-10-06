package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const ampConfigWithPort = `# Webserver.Port - NEVER CHANGE THIS SETTING MANUALLY!
Webserver.Port=8083
Webserver.IPBinding=127.0.0.1
`

func TestParseAMPWebserverPort(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		conf string
		want int
	}{
		{"present", ampConfigWithPort, 8083},
		{"spaces around equals", "Webserver.Port = 8090\n", 8090},
		{"crlf line endings", "Webserver.Port=8085\r\nOther=1\r\n", 8085},
		{"commented out only", "# Webserver.Port=8083\n", 0},
		{"missing", "Webserver.IPBinding=0.0.0.0\n", 0},
		{"not a number", "Webserver.Port=abc\n", 0},
		{"out of range", "Webserver.Port=70000\n", 0},
		{"empty", "", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := parseAMPWebserverPort(tc.conf); got != tc.want {
				t.Errorf("parseAMPWebserverPort() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestAmpInstanceConfigPath(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		c    *ampControl
		want string
	}{
		{"conventional home", &ampControl{instance: "DuneAwakening01", ampUser: "amp"},
			"/home/amp/.ampdata/instances/DuneAwakening01/AMPConfig.conf"},
		{"default amp user", &ampControl{instance: "Dune01"},
			"/home/amp/.ampdata/instances/Dune01/AMPConfig.conf"},
		{"derived from configured ini dir", &ampControl{instance: "Dune01", ampUser: "amp",
			iniDir: "/srv/amp/.ampdata/instances/Dune01/duneawakening/server/state"},
			"/srv/amp/.ampdata/instances/Dune01/AMPConfig.conf"},
		{"no instance", &ampControl{ampUser: "amp"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.c.ampInstanceConfigPath(); got != tc.want {
				t.Errorf("ampInstanceConfigPath() = %q, want %q", got, tc.want)
			}
		})
	}
}

// A remote amp_api_host (#284 split topology) points at another machine, so the
// local instance config says nothing about that port: the configured port wins.
func TestAmpResolveAPIPort_RemoteHostUsesConfiguredPort(t *testing.T) {
	t.Parallel()
	exec := &fnExecutor{fn: func(cmd string) (string, error) {
		t.Fatalf("remote amp_api_host must not read AMPConfig.conf; got cmd %q", cmd)
		return "", nil
	}}
	c := &ampControl{instance: "Dune01", ampUser: "amp", apiHost: "192.168.50.171", apiPort: 9000}
	if got := c.resolveAPIPort(exec); got != 9000 {
		t.Errorf("resolveAPIPort() = %d, want 9000", got)
	}
}

// Server records store amp_api_port=8081 from the defaults fill, so a stored
// port can't be told apart from the default. On loopback the instance's own
// Webserver.Port is where the API actually listens, so it wins (#340).
func TestAmpResolveAPIPort_LoopbackInstanceConfigBeatsStoredPort(t *testing.T) {
	t.Parallel()
	for _, host := range []string{"", "127.0.0.1", "localhost", "::1"} {
		t.Run("host="+host, func(t *testing.T) {
			t.Parallel()
			exec := &fnExecutor{fn: func(string) (string, error) { return ampConfigWithPort, nil }}
			c := &ampControl{instance: "Dune01", ampUser: "amp", apiHost: host, apiPort: 8081}
			if got := c.resolveAPIPort(exec); got != 8083 {
				t.Errorf("resolveAPIPort() = %d, want 8083", got)
			}
		})
	}
}

func TestAmpResolveAPIPort_LoopbackFallsBackToStoredPort(t *testing.T) {
	t.Parallel()
	exec := &fnExecutor{fn: func(string) (string, error) { return "", errors.New("permission denied") }}
	c := &ampControl{instance: "Dune01", ampUser: "amp", apiHost: "127.0.0.1", apiPort: 9000}
	if got := c.resolveAPIPort(exec); got != 9000 {
		t.Errorf("resolveAPIPort() = %d, want 9000", got)
	}
}

func TestAmpResolveAPIPort_ReadsInstanceConfig(t *testing.T) {
	t.Parallel()
	var cmds []string
	exec := &fnExecutor{fn: func(cmd string) (string, error) {
		cmds = append(cmds, cmd)
		if strings.Contains(cmd, "/home/amp/.ampdata/instances/Dune01/AMPConfig.conf") {
			return ampConfigWithPort, nil
		}
		return "", errors.New("unexpected")
	}}
	c := &ampControl{instance: "Dune01", ampUser: "amp"}
	if got := c.resolveAPIPort(exec); got != 8083 {
		t.Errorf("resolveAPIPort() = %d, want 8083 (cmds: %v)", got, cmds)
	}
}

// When dune-admin runs as its own user, the host AMPConfig.conf under the amp
// user's home is unreadable and the narrow sudoers doesn't grant cat. In
// container mode the instance root is mounted at /AMP, reachable through the
// already-granted `<runtime> exec`.
func TestAmpResolveAPIPort_ContainerModeReadsInContainerConfig(t *testing.T) {
	t.Parallel()
	exec := &fnExecutor{fn: func(cmd string) (string, error) {
		if strings.Contains(cmd, "exec AMP_X") && strings.Contains(cmd, "/AMP/AMPConfig.conf") {
			return ampConfigWithPort, nil
		}
		return "", errors.New("permission denied")
	}}
	c := ampSettingsControl()
	c.instance = "Dune01"
	if got := c.resolveAPIPort(exec); got != 8083 {
		t.Errorf("resolveAPIPort() = %d, want 8083", got)
	}
}

func TestAmpResolveAPIPort_ContainerModeFallsBackToHostConfig(t *testing.T) {
	t.Parallel()
	exec := &fnExecutor{fn: func(cmd string) (string, error) {
		if strings.Contains(cmd, "exec AMP_X") {
			return "", errors.New("container not running")
		}
		if strings.Contains(cmd, "/home/amp/.ampdata/instances/Dune01/AMPConfig.conf") {
			return ampConfigWithPort, nil
		}
		return "", errors.New("unexpected")
	}}
	c := ampSettingsControl()
	c.instance = "Dune01"
	if got := c.resolveAPIPort(exec); got != 8083 {
		t.Errorf("resolveAPIPort() = %d, want 8083", got)
	}
}

func TestAmpResolveAPIPort_FallsBackToDefault(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		c    *ampControl
		fn   func(string) (string, error)
	}{
		{"unreadable config", &ampControl{instance: "Dune01", ampUser: "amp"},
			func(string) (string, error) { return "", errors.New("permission denied") }},
		{"config without port", &ampControl{instance: "Dune01", ampUser: "amp"},
			func(string) (string, error) { return "Webserver.IPBinding=0.0.0.0\n", nil }},
		{"no instance configured", &ampControl{ampUser: "amp"},
			func(cmd string) (string, error) { return "", errors.New("must not be called: " + cmd) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.c.resolveAPIPort(&fnExecutor{fn: tc.fn}); got != 0 {
				t.Errorf("resolveAPIPort() = %d, want 0 (client default)", got)
			}
		})
	}
}

// TestAmpReadServerSettings_UsesInstancePort is the #340 regression: with no
// amp_api_port configured, dune-admin assumed 8081 and talked to whichever AMP
// instance owns that port. SSO logins succeed on any instance, so every Dune
// node came back "No such node". The API must target the Dune instance's own
// Webserver.Port.
func TestAmpReadServerSettings_UsesInstancePort(t *testing.T) {
	t.Parallel()
	var apiCmds []string
	exec := &fnExecutor{fn: func(cmd string) (string, error) {
		switch {
		case strings.Contains(cmd, "AMPConfig.conf"):
			return ampConfigWithPort, nil
		case strings.Contains(cmd, "Core/Login"):
			apiCmds = append(apiCmds, cmd)
			return `{"success":true,"sessionID":"sess"}`, nil
		case strings.Contains(cmd, "Core/GetConfig"):
			apiCmds = append(apiCmds, cmd)
			return `{"CurrentValue":"3.0"}`, nil
		}
		t.Fatalf("unexpected cmd: %q", cmd)
		return "", nil
	}}
	c := ampSettingsControl()
	c.instance = "DuneAwakening01"

	got, err := c.readServerSettings(context.Background(), exec, []string{"ConsoleVariables.Dune.GlobalMiningOutputMultiplier"})
	if err != nil {
		t.Fatalf("readServerSettings: %v", err)
	}
	if got["ConsoleVariables.Dune.GlobalMiningOutputMultiplier"] != "3.0" {
		t.Errorf("mining = %q, want 3.0", got["ConsoleVariables.Dune.GlobalMiningOutputMultiplier"])
	}
	if len(apiCmds) == 0 {
		t.Fatal("no AMP API calls made")
	}
	for _, cmd := range apiCmds {
		if !strings.Contains(cmd, "127.0.0.1:8083/API/") {
			t.Errorf("API call not sent to instance port 8083: %q", cmd)
		}
	}
}

func TestAmpWriteServerSettings_UsesInstancePort(t *testing.T) {
	t.Parallel()
	var apiCmds []string
	exec := &fnExecutor{fn: func(cmd string) (string, error) {
		switch {
		case strings.Contains(cmd, "AMPConfig.conf"):
			return ampConfigWithPort, nil
		case strings.Contains(cmd, "Core/Login"):
			apiCmds = append(apiCmds, cmd)
			return `{"success":true,"sessionID":"sess"}`, nil
		case strings.Contains(cmd, "Core/SetConfig"):
			apiCmds = append(apiCmds, cmd)
			return `{"Status":true}`, nil
		}
		t.Fatalf("unexpected cmd: %q", cmd)
		return "", nil
	}}
	c := ampSettingsControl()
	c.instance = "DuneAwakening01"

	if err := c.writeServerSettings(context.Background(), exec, map[string]string{"WorldTitle": "x"}); err != nil {
		t.Fatalf("writeServerSettings: %v", err)
	}
	for _, cmd := range apiCmds {
		if !strings.Contains(cmd, "127.0.0.1:8083/API/") {
			t.Errorf("API call not sent to instance port 8083: %q", cmd)
		}
	}
}
