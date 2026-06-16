package main

import "testing"

func TestRedactLine(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string // substring that MUST appear (redacted form)
		gone string // substring that MUST NOT appear
	}{
		{"ipv4", "dialing 192.168.0.59:8080 now", "[redacted-host]", "192.168.0.59"},
		{"bearer", `Authorization: Bearer abc.def.ghi`, "[redacted-token]", "abc.def.ghi"},
		{"service token", `ServiceAuthToken=SECRETVALUE123`, "[redacted-token]", "SECRETVALUE123"},
		{"kv password", `password=hunter2 extra`, "[redacted-token]", "hunter2"},
		{"ssh target", `ssh amp@192.168.0.59`, "[redacted-host]", "amp@192.168.0.59"},
		{"home path", `/Users/icehunter/.dune-admin/config.yaml`, "[redacted-path]", "icehunter"},
		{"account id", `account_id=1099511628800 done`, "[redacted-id]", "1099511628800"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := redactLine(c.in)
			if c.want != "" && !contains(got, c.want) {
				t.Errorf("redactLine(%q) = %q, want substring %q", c.in, got, c.want)
			}
			if c.gone != "" && contains(got, c.gone) {
				t.Errorf("redactLine(%q) = %q, must not contain %q", c.in, got, c.gone)
			}
		})
	}
}

func TestRedactLineLeavesSafeTextAlone(t *testing.T) {
	in := `level=info component=handlers msg="server started"`
	if got := redactLine(in); got != in {
		t.Errorf("redactLine altered safe text: %q -> %q", in, got)
	}
}

func TestRedactLineIPv6(t *testing.T) {
	cases := []struct {
		in   string
		gone string
	}{
		{"connecting to [2001:db8::1]:8080 ok", "2001:db8::1"},
		{"peer fe80::1ff:fe23:4567:890a down", "fe80::1ff:fe23:4567:890a"},
		{"loopback ::1 reached", "::1"},
	}
	for _, c := range cases {
		got := redactLine(c.in)
		if contains(got, c.gone) {
			t.Errorf("redactLine(%q) = %q, must not contain %q", c.in, got, c.gone)
		}
		if !contains(got, "[redacted-host]") {
			t.Errorf("redactLine(%q) = %q, want [redacted-host]", c.in, got)
		}
	}
}
