package main

import (
	"reflect"
	"strings"
	"testing"
)

// Golden fixture captured from a real `sudo -u amp ampinstmgr -l` run on an
// AMP host running both an ADS instance (the AMP control panel itself) and a
// GenericModule Dune instance. The parser should filter out ADS and surface
// only the game instance.
const ampInstmgrSampleOutput = `[Info/1] AMP Instance Manager v2.7.2.8 built 20/05/2026 06:54
[Info/1] Stream: Mainline / Release - built by CUBECODERS/buildbot on CCL-DEV
cannot chdir to /home/test: Permission denied
Instance ID        │ 88fe1020-71ed-4789-b390-c03a165f5630
Module             │ ADS
Instance Name      │ ADS01
Friendly Name      │ ADS01
URL                │ http://127.0.0.1:8080/
Running            │ Yes
Runs in Container  │ No
Runs as Shared     │ No
Start on Boot      │ Yes
AMP Version        │ 2.7.2.8
Release Stream     │ Mainline
Data Path          │ /home/amp/.ampdata/instances/ADS01

Instance ID        │ 0f8247da-f1c9-4898-a806-8017beeb15e7
Module             │ GenericModule
Instance Name      │ DuneTest01
Friendly Name      │ DuneTest
URL                │ http://127.0.0.1:8081/
Running            │ No
Runs in Container  │ Yes
Runs as Shared     │ No
Start on Boot      │ Yes
AMP Version        │ 2.7.2.8
Release Stream     │ Mainline
Data Path          │ /home/amp/.ampdata/instances/DuneTest01
`

func TestParseAmpInstmgrOutput_FiltersADSAndKeepsGame(t *testing.T) {
	got := parseAmpInstmgrOutput([]byte(ampInstmgrSampleOutput))
	want := []ampInstance{
		{
			Name:        "DuneTest01",
			Module:      "GenericModule",
			Running:     false,
			InContainer: true,
			DataPath:    "/home/amp/.ampdata/instances/DuneTest01",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseAmpInstmgrOutput: got %#v, want %#v", got, want)
	}
}

func TestParseAmpInstmgrOutput_MultipleGameInstances(t *testing.T) {
	in := `Instance Name      │ DuneLive01
Module             │ DuneAwakening
Running            │ Yes
Runs in Container  │ No
Data Path          │ /home/amp/.ampdata/instances/DuneLive01

Instance Name      │ DunePTS
Module             │ DuneAwakening
Running            │ No
Runs in Container  │ Yes
Data Path          │ /home/amp/.ampdata/instances/DunePTS
`
	got := parseAmpInstmgrOutput([]byte(in))
	if len(got) != 2 {
		t.Fatalf("expected 2 instances, got %d: %#v", len(got), got)
	}
	if got[0].Name != "DuneLive01" || !got[0].Running || got[0].InContainer {
		t.Errorf("first instance wrong: %#v", got[0])
	}
	if got[1].Name != "DunePTS" || got[1].Running || !got[1].InContainer {
		t.Errorf("second instance wrong: %#v", got[1])
	}
}

func TestParseAmpInstmgrOutput_EmptyOrGarbage(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"banner only", "[Info/1] AMP Instance Manager v2.7.2.8\n"},
		{"unrelated lines", "hello\nworld\nno separators here\n"},
		{"missing instance name", "Module             │ DuneAwakening\nRunning            │ Yes\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseAmpInstmgrOutput([]byte(c.in))
			if len(got) != 0 {
				t.Errorf("expected 0 instances, got %d: %#v", len(got), got)
			}
		})
	}
}

func TestParseAmpInstmgrOutput_AsciiPipeFallback(t *testing.T) {
	// Future-proofing: if ampinstmgr ever drops Unicode in scripted mode and
	// switches to plain ASCII pipes, we should still parse it.
	in := `Instance Name      | DuneFallback
Module             | DuneAwakening
Running            | Yes
Runs in Container  | Yes
Data Path          | /home/amp/.ampdata/instances/DuneFallback
`
	got := parseAmpInstmgrOutput([]byte(in))
	if len(got) != 1 || got[0].Name != "DuneFallback" {
		t.Errorf("ASCII-pipe parsing failed: %#v", got)
	}
}

func TestSplitAmpKV(t *testing.T) {
	cases := []struct {
		line string
		key  string
		val  string
		ok   bool
	}{
		{"Instance Name      │ DuneTest01", "Instance Name", "DuneTest01", true},
		{"Module | DuneAwakening", "Module", "DuneAwakening", true},
		{"no separator here", "", "", false},
		{"  │  ", "", "", true}, // edge case: empty key/val but valid split
	}
	for _, c := range cases {
		t.Run(c.line, func(t *testing.T) {
			k, v, ok := splitAmpKV(c.line)
			if ok != c.ok || k != c.key || v != c.val {
				t.Errorf("splitAmpKV(%q) = (%q, %q, %v); want (%q, %q, %v)",
					c.line, k, v, ok, c.key, c.val, c.ok)
			}
		})
	}
}

func TestSummarizeInstance(t *testing.T) {
	got := summarizeInstance(ampInstance{
		Name: "DuneTest01", Module: "GenericModule", Running: true, InContainer: true,
	})
	for _, want := range []string{"DuneTest01", "GenericModule", "container", "running"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary %q missing %q", got, want)
		}
	}
}
