package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// TestFindServerSetForPartition covers the partition→ServerSet lookup (#185):
// each ServerSet CR carries spec.partitions (a []int) naming the partition
// index(es) it backs. The kubectl output is `name|space-separated-indexes`
// per line (built via jsonpath in production).
func TestFindServerSetForPartition(t *testing.T) {
	tests := []struct {
		name      string
		out       string
		execErr   error
		partition int
		want      string
		wantErr   bool
	}{
		{
			name:      "match found",
			out:       "sh-abc-sg-hagga-1|1 \nsh-abc-sg-survival-2|2 \n",
			partition: 2,
			want:      "sh-abc-sg-survival-2",
		},
		{
			name:      "no match",
			out:       "sh-abc-sg-hagga-1|1 \n",
			partition: 9,
			wantErr:   true,
		},
		{
			name:      "empty output",
			out:       "",
			partition: 1,
			wantErr:   true,
		},
		{
			name:      "malformed line skipped, later match still found",
			out:       "garbage-no-pipe\nsh-abc-sg-hagga-1|1 \n",
			partition: 1,
			want:      "sh-abc-sg-hagga-1",
		},
		{
			name:      "exec error propagates",
			out:       "",
			execErr:   context.DeadlineExceeded,
			partition: 1,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := &fnExecutor{fn: func(cmd string) (string, error) {
				if !strings.Contains(cmd, "get serversets") {
					t.Fatalf("expected a `get serversets` command, got %q", cmd)
				}
				return tt.out, tt.execErr
			}}
			got, err := findServerSetForPartition(exec, "kubectl", "ns1", tt.partition)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got name %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("findServerSetForPartition() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestFindPartitionPod covers resolving the concrete pod name backing a
// ServerSet, rather than assuming a StatefulSet-ordinal "-0" suffix (#185).
func TestFindPartitionPod(t *testing.T) {
	tests := []struct {
		name        string
		ssOut       string
		ssErr       error
		podOut      string
		podErr      error
		partition   int
		wantPod     string
		wantErr     bool
		wantErrText string
	}{
		{
			name:      "resolves pod for matched serverset",
			ssOut:     "sh-abc-sg-survival-1|1 \n",
			podOut:    "sh-abc-sg-survival-1-0\n",
			partition: 1,
			wantPod:   "sh-abc-sg-survival-1-0",
		},
		{
			name:        "no serverset for partition",
			ssOut:       "sh-abc-sg-survival-1|1 \n",
			partition:   7,
			wantErr:     true,
			wantErrText: "no ServerSet found for partition 7",
		},
		{
			name:        "serverset found but no pod running",
			ssOut:       "sh-abc-sg-survival-1|1 \n",
			podOut:      "",
			partition:   1,
			wantErr:     true,
			wantErrText: "no pod found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := &fnExecutor{fn: func(cmd string) (string, error) {
				switch {
				case strings.Contains(cmd, "get serversets"):
					return tt.ssOut, tt.ssErr
				case strings.Contains(cmd, "get pods"):
					return tt.podOut, tt.podErr
				default:
					t.Fatalf("unexpected command: %q", cmd)
					return "", nil
				}
			}}
			pod, err := findPartitionPod(exec, "kubectl", "ns1", tt.partition)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got pod %q", pod)
				}
				if tt.wantErrText != "" && !strings.Contains(err.Error(), tt.wantErrText) {
					t.Errorf("error = %q, want substring %q", err.Error(), tt.wantErrText)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pod != tt.wantPod {
				t.Errorf("pod = %q, want %q", pod, tt.wantPod)
			}
		})
	}
}

// TestBuildServerRestartManifest covers the ServerRestart CR JSON generated
// for a per-partition restart (#185) — mode "Pods" targeting a single
// resolved pod, never the whole-Battlegroup "spec.stop" primitive.
func TestBuildServerRestartManifest(t *testing.T) {
	out, err := buildServerRestartManifest("funcom-seabass-mybg", "mybg", "sh-abc-sg-survival-1-0", 1)
	if err != nil {
		t.Fatalf("buildServerRestartManifest: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("manifest is not valid JSON: %v\n%s", err, out)
	}
	if doc["kind"] != "ServerRestart" {
		t.Errorf("kind = %v, want ServerRestart", doc["kind"])
	}
	if doc["apiVersion"] != serverRestartAPIVersion {
		t.Errorf("apiVersion = %v, want %v", doc["apiVersion"], serverRestartAPIVersion)
	}
	meta, _ := doc["metadata"].(map[string]any)
	if meta["namespace"] != "funcom-seabass-mybg" {
		t.Errorf("metadata.namespace = %v, want funcom-seabass-mybg", meta["namespace"])
	}
	if meta["generateName"] == "" || meta["generateName"] == nil {
		t.Error("metadata.generateName must be set — ServerRestart objects are one-shot and never reused")
	}
	spec, _ := doc["spec"].(map[string]any)
	if spec["battleGroup"] != "mybg" {
		t.Errorf("spec.battleGroup = %v, want mybg", spec["battleGroup"])
	}
	if spec["mode"] != "Pods" {
		t.Errorf("spec.mode = %v, want Pods", spec["mode"])
	}
	if spec["pod"] != "sh-abc-sg-survival-1-0" {
		t.Errorf("spec.pod = %v, want sh-abc-sg-survival-1-0", spec["pod"])
	}
	reason, _ := spec["reason"].(string)
	if !strings.Contains(reason, "partition 1") {
		t.Errorf("spec.reason = %q, want it to mention partition 1", reason)
	}
}

// TestKubectlControl_RestartPartition_Success covers the full flow: resolve
// ServerSet → resolve pod → apply a ServerRestart CR (mode=Pods), never the
// whole-Battlegroup spec.stop patch used by ExecCommand("restart").
func TestKubectlControl_RestartPartition_Success(t *testing.T) {
	var applyCmd string
	exec := &fnExecutor{fn: func(cmd string) (string, error) {
		switch {
		case strings.Contains(cmd, "get serversets"):
			return "sh-abc-sg-survival-1|1 \n", nil
		case strings.Contains(cmd, "get pods"):
			return "sh-abc-sg-survival-1-0\n", nil
		case strings.Contains(cmd, "apply"):
			applyCmd = cmd
			return `serverrestart.igw.funcom.com/dune-admin-restart-abc123 created`, nil
		default:
			t.Fatalf("unexpected command: %q", cmd)
			return "", nil
		}
	}}

	c := &kubectlControl{namespace: "funcom-seabass-mybg"}
	out, err := c.RestartPartition(context.Background(), exec, 1)
	if err != nil {
		t.Fatalf("RestartPartition: %v", err)
	}
	if !strings.Contains(out, "created") {
		t.Errorf("output = %q, want it to reflect the apply result", out)
	}
	if !strings.Contains(applyCmd, "apply") || !strings.Contains(applyCmd, "-n funcom-seabass-mybg") {
		t.Fatalf("apply command = %q, missing namespace/apply", applyCmd)
	}
	if !strings.Contains(applyCmd, `"mode":"Pods"`) {
		t.Errorf("apply command = %q, want mode=Pods (never whole-Battlegroup stop)", applyCmd)
	}
	if !strings.Contains(applyCmd, `"pod":"sh-abc-sg-survival-1-0"`) {
		t.Errorf("apply command = %q, missing resolved pod name", applyCmd)
	}
	if strings.Contains(applyCmd, `"spec":{"stop"`) {
		t.Fatalf("apply command = %q must never patch the whole-Battlegroup spec.stop field", applyCmd)
	}
}

// TestKubectlControl_RestartPartition_ServerSetNotFound covers the case where
// no ServerSet claims the requested partition — must fail before ever
// touching the cluster's apply path.
func TestKubectlControl_RestartPartition_ServerSetNotFound(t *testing.T) {
	applied := false
	exec := &fnExecutor{fn: func(cmd string) (string, error) {
		if strings.Contains(cmd, "get serversets") {
			return "sh-abc-sg-hagga-1|1 \n", nil
		}
		if strings.Contains(cmd, "apply") {
			applied = true
		}
		return "", nil
	}}
	c := &kubectlControl{namespace: "funcom-seabass-mybg"}
	if _, err := c.RestartPartition(context.Background(), exec, 99); err == nil {
		t.Fatal("expected error for a partition with no matching ServerSet")
	}
	if applied {
		t.Fatal("must not apply a ServerRestart when the partition could not be resolved to a pod")
	}
}

// TestKubectlControl_RestartPartition_ApplyError covers the ServerRestart CR
// apply failing — the error and raw output must both surface to the caller.
func TestKubectlControl_RestartPartition_ApplyError(t *testing.T) {
	exec := &fnExecutor{fn: func(cmd string) (string, error) {
		switch {
		case strings.Contains(cmd, "get serversets"):
			return "sh-abc-sg-survival-1|1 \n", nil
		case strings.Contains(cmd, "get pods"):
			return "sh-abc-sg-survival-1-0\n", nil
		case strings.Contains(cmd, "apply"):
			return "error: some cluster problem", errors.New("apply failed")
		default:
			return "", nil
		}
	}}
	c := &kubectlControl{namespace: "funcom-seabass-mybg"}
	out, err := c.RestartPartition(context.Background(), exec, 1)
	if err == nil {
		t.Fatal("expected error when apply fails")
	}
	if !strings.Contains(out, "cluster problem") && !strings.Contains(err.Error(), "cluster problem") {
		t.Errorf("expected the raw apply output to surface somewhere, err=%v out=%q", err, out)
	}
}
