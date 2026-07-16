package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestKubectlControl_GetStatus_PerPartitionAge covers #277: Age previously came
// from a single battlegroup-wide startTimestamp, so every ServerRow (one per
// map/dimension/partition) reported identical uptime regardless of how long
// that specific map process had actually been running. Each serverstats
// object carries its own metadata.creationTimestamp (confirmed against the
// live serverstats.igw.funcom.com CRD — it's the same field the CRD's own
// "Age" printer column uses), so GetStatus must source per-row age from that
// instead of the battlegroup-level timestamp.
func TestKubectlControl_GetStatus_PerPartitionAge(t *testing.T) {
	t.Parallel()

	exec := &fnExecutor{fn: func(cmd string) (string, error) {
		switch {
		case strings.Contains(cmd, "get battlegroups") && strings.Contains(cmd, "spec.title"):
			// Battlegroup-level startTimestamp is much older than either
			// per-map server — proves rows no longer inherit it directly.
			return `MyBG|Running|Ready|2026-01-01T00:00:00Z`, nil
		case strings.Contains(cmd, "get battlegroups") && strings.Contains(cmd, "partitionIndex"):
			return "", nil // no gamePort data needed for this test
		case strings.Contains(cmd, "get serverstats"):
			return strings.Join([]string{
				"HaggaBasin|Alraab|0|1|Playing|true|12|2026-06-13T11:00:00Z",
				"DeepDesert|Abbir|0|2|Playing|true|3|2026-06-13T10:00:00Z",
			}, "\n"), nil
		default:
			return "", nil
		}
	}}

	c := &kubectlControl{namespace: "funcom-seabass-mybg"}
	status, err := c.GetStatus(context.Background(), exec)
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if len(status.Servers) != 2 {
		t.Fatalf("len(Servers) = %d, want 2", len(status.Servers))
	}

	byPartition := map[int]ServerRow{}
	for _, s := range status.Servers {
		byPartition[s.Partition] = s
	}

	hagga, ok := byPartition[1]
	if !ok {
		t.Fatalf("missing partition 1 (HaggaBasin) row")
	}
	deepDesert, ok := byPartition[2]
	if !ok {
		t.Fatalf("missing partition 2 (DeepDesert) row")
	}

	if hagga.AgeSeconds == deepDesert.AgeSeconds {
		t.Fatalf("expected different AgeSeconds per partition, both got %d", hagga.AgeSeconds)
	}
	// DeepDesert's serverstats object is older (10:00 vs 11:00 same day) so it
	// must report a larger AgeSeconds than HaggaBasin's.
	if deepDesert.AgeSeconds <= hagga.AgeSeconds {
		t.Fatalf("DeepDesert.AgeSeconds = %d, want > HaggaBasin.AgeSeconds = %d", deepDesert.AgeSeconds, hagga.AgeSeconds)
	}
	// Neither row should equal the stale battlegroup-wide age computed from
	// the 2026-01-01 startTimestamp (which would be a much larger number,
	// since that date is many months before the per-row timestamps above).
	bgWideAge := ageSecondsFromStartTime("2026-01-01T00:00:00Z", time.Now())
	if hagga.AgeSeconds == bgWideAge || deepDesert.AgeSeconds == bgWideAge {
		t.Fatalf("a row's AgeSeconds still equals the stale battlegroup-wide age %d — not sourced per-row", bgWideAge)
	}
}
