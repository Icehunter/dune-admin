package main

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
)

// ── embed builder ─────────────────────────────────────────────────────────────

func TestBuildStatusEmbed(t *testing.T) {
	tests := []struct {
		name        string
		data        statusEmbedData
		wantTitleHt string   // substring expected in the title
		wantInDesc  []string // substrings expected in description or fields
		wantColor   int
	}{
		{
			name: "online with maps and counts",
			data: statusEmbedData{
				State:         serverStateOnline,
				CurrentOnline: 5,
				TotalPlayers:  120,
				UniquePlayers: 42,
				Maps: []mapPlayerCount{
					{Map: "Hagga Basin", Players: 3},
					{Map: "Deep Desert", Players: 2},
				},
			},
			wantInDesc: []string{"Hagga Basin", "Deep Desert", "3", "2", "42", "5", "120"},
			wantColor:  statusColorOnline,
		},
		{
			name: "offline shows offline state and no maps",
			data: statusEmbedData{
				State: serverStateOffline,
			},
			wantInDesc: []string{"Offline"},
			wantColor:  statusColorOffline,
		},
		{
			name: "booting state",
			data: statusEmbedData{
				State: serverStateBooting,
			},
			wantInDesc: []string{"Booting"},
			wantColor:  statusColorBooting,
		},
		{
			name: "restarting state",
			data: statusEmbedData{
				State: serverStateRestarting,
			},
			wantInDesc: []string{"Restarting"},
			wantColor:  statusColorRestarting,
		},
		{
			name: "online but no maps active",
			data: statusEmbedData{
				State:         serverStateOnline,
				CurrentOnline: 0,
				UniquePlayers: 7,
			},
			wantInDesc: []string{"Online", "7"},
			wantColor:  statusColorOnline,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			embed := buildStatusEmbed(tt.data, time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC))
			if embed == nil {
				t.Fatal("buildStatusEmbed returned nil")
			}
			if embed.Color != tt.wantColor {
				t.Errorf("color = %d, want %d", embed.Color, tt.wantColor)
			}
			// Flatten the searchable text of the embed.
			var sb strings.Builder
			sb.WriteString(embed.Title)
			sb.WriteString("\n")
			sb.WriteString(embed.Description)
			for _, f := range embed.Fields {
				sb.WriteString("\n")
				sb.WriteString(f.Name)
				sb.WriteString("\n")
				sb.WriteString(f.Value)
			}
			haystack := sb.String()
			for _, want := range tt.wantInDesc {
				if !strings.Contains(haystack, want) {
					t.Errorf("embed text %q does not contain %q", haystack, want)
				}
			}
		})
	}
}

func TestBuildStatusEmbed_StableTimestamp(t *testing.T) {
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	embed := buildStatusEmbed(statusEmbedData{State: serverStateOnline}, now)
	if embed.Timestamp == "" {
		t.Error("expected non-empty timestamp")
	}
}

// ── post-or-edit ──────────────────────────────────────────────────────────────

// fakeStatusSession records calls and lets tests script the returned IDs/errors.
type fakeStatusSession struct {
	mu sync.Mutex

	sendChannelID string
	sendReturnID  string
	sendErr       error
	sendCalls     int

	editChannelID string
	editMsgID     string
	editErr       error
	editCalls     int
}

func (f *fakeStatusSession) ChannelMessageSendEmbed(channelID string, _ *discordgo.MessageEmbed) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sendCalls++
	f.sendChannelID = channelID
	if f.sendErr != nil {
		return "", f.sendErr
	}
	return f.sendReturnID, nil
}

func (f *fakeStatusSession) ChannelMessageEditEmbed(channelID, messageID string, _ *discordgo.MessageEmbed) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.editCalls++
	f.editChannelID = channelID
	f.editMsgID = messageID
	return f.editErr
}

// memStatusStore is an in-memory statusMessageStore for tests.
type memStatusStore struct {
	mu        sync.Mutex
	channelID string
	messageID string
	loadErr   error
	saveErr   error
	saveCalls int
}

func (m *memStatusStore) loadStatusMessage() (channelID, messageID string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.channelID, m.messageID, m.loadErr
}

func (m *memStatusStore) saveStatusMessage(channelID, messageID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.saveCalls++
	if m.saveErr != nil {
		return m.saveErr
	}
	m.channelID = channelID
	m.messageID = messageID
	return nil
}

func TestPostOrEditStatusEmbed_SendsWhenNoStoredID(t *testing.T) {
	sess := &fakeStatusSession{sendReturnID: "msg-1"}
	store := &memStatusStore{}
	embed := &discordgo.MessageEmbed{Title: "t"}

	err := postOrEditStatusEmbed(sess, store, "chan-1", embed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sess.sendCalls != 1 {
		t.Errorf("send calls = %d, want 1", sess.sendCalls)
	}
	if sess.editCalls != 0 {
		t.Errorf("edit calls = %d, want 0", sess.editCalls)
	}
	if store.channelID != "chan-1" || store.messageID != "msg-1" {
		t.Errorf("persisted (%q,%q), want (chan-1,msg-1)", store.channelID, store.messageID)
	}
}

func TestPostOrEditStatusEmbed_EditsWhenStoredIDMatches(t *testing.T) {
	sess := &fakeStatusSession{}
	store := &memStatusStore{channelID: "chan-1", messageID: "msg-1"}
	embed := &discordgo.MessageEmbed{Title: "t"}

	err := postOrEditStatusEmbed(sess, store, "chan-1", embed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sess.editCalls != 1 {
		t.Errorf("edit calls = %d, want 1", sess.editCalls)
	}
	if sess.sendCalls != 0 {
		t.Errorf("send calls = %d, want 0", sess.sendCalls)
	}
	if sess.editMsgID != "msg-1" {
		t.Errorf("edited msg id = %q, want msg-1", sess.editMsgID)
	}
	if store.saveCalls != 0 {
		t.Errorf("should not re-save on a successful edit, got %d saves", store.saveCalls)
	}
}

func TestPostOrEditStatusEmbed_ResendsWhenEditNotFound(t *testing.T) {
	// Stored message was deleted in Discord: edit fails, we should re-send and
	// persist the new ID.
	sess := &fakeStatusSession{
		editErr:      &discordgo.RESTError{Message: &discordgo.APIErrorMessage{Code: discordgo.ErrCodeUnknownMessage}},
		sendReturnID: "msg-2",
	}
	store := &memStatusStore{channelID: "chan-1", messageID: "old-msg"}
	embed := &discordgo.MessageEmbed{Title: "t"}

	err := postOrEditStatusEmbed(sess, store, "chan-1", embed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sess.editCalls != 1 {
		t.Errorf("edit calls = %d, want 1", sess.editCalls)
	}
	if sess.sendCalls != 1 {
		t.Errorf("send calls = %d, want 1 (resend after not-found)", sess.sendCalls)
	}
	if store.messageID != "msg-2" {
		t.Errorf("persisted msg id = %q, want msg-2", store.messageID)
	}
}

func TestPostOrEditStatusEmbed_ResendsWhenChannelChanged(t *testing.T) {
	// Stored channel differs from the configured channel → treat as a fresh post.
	sess := &fakeStatusSession{sendReturnID: "msg-3"}
	store := &memStatusStore{channelID: "old-chan", messageID: "msg-1"}
	embed := &discordgo.MessageEmbed{Title: "t"}

	err := postOrEditStatusEmbed(sess, store, "new-chan", embed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sess.sendCalls != 1 {
		t.Errorf("send calls = %d, want 1", sess.sendCalls)
	}
	if store.channelID != "new-chan" || store.messageID != "msg-3" {
		t.Errorf("persisted (%q,%q), want (new-chan,msg-3)", store.channelID, store.messageID)
	}
}

func TestPostOrEditStatusEmbed_SendErrorPropagates(t *testing.T) {
	sess := &fakeStatusSession{sendErr: errors.New("boom")}
	store := &memStatusStore{}
	err := postOrEditStatusEmbed(sess, store, "chan-1", &discordgo.MessageEmbed{})
	if err == nil {
		t.Fatal("expected error to propagate")
	}
	if store.saveCalls != 0 {
		t.Errorf("must not persist on send failure, got %d saves", store.saveCalls)
	}
}

// ── meta store (SQLite persistence) ───────────────────────────────────────────

func TestSqliteStatusStore_RoundTrip(t *testing.T) {
	db, err := openUnifiedStore(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = db.Close() }()

	store := newSqliteStatusStore(db)

	// Fresh store: empty values, no error.
	ch, msg, err := store.loadStatusMessage()
	if err != nil {
		t.Fatalf("load on empty: %v", err)
	}
	if ch != "" || msg != "" {
		t.Errorf("empty load = (%q,%q), want empty", ch, msg)
	}

	if err := store.saveStatusMessage("chan-9", "msg-9"); err != nil {
		t.Fatalf("save: %v", err)
	}
	ch, msg, err = store.loadStatusMessage()
	if err != nil {
		t.Fatalf("load after save: %v", err)
	}
	if ch != "chan-9" || msg != "msg-9" {
		t.Errorf("round trip = (%q,%q), want (chan-9,msg-9)", ch, msg)
	}

	// Overwrite.
	if err := store.saveStatusMessage("chan-10", "msg-10"); err != nil {
		t.Fatalf("overwrite save: %v", err)
	}
	ch, msg, _ = store.loadStatusMessage()
	if ch != "chan-10" || msg != "msg-10" {
		t.Errorf("after overwrite = (%q,%q), want (chan-10,msg-10)", ch, msg)
	}
}

// ── 24h unique players ────────────────────────────────────────────────────────

func TestCountUniquePlayers24h(t *testing.T) {
	db, err := openUnifiedStore(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = db.Close() }()

	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-2 * time.Hour).Format(time.RFC3339)
	alsoRecent := now.Add(-23 * time.Hour).Format(time.RFC3339)
	old := now.Add(-48 * time.Hour).Format(time.RFC3339)

	// account 1 appears twice (recent + old), account 2 once recent, account 3 old only.
	insert := func(acct int64, started string) {
		if _, e := db.Exec(`INSERT INTO play_sessions(account_id, started_at) VALUES(?, ?)`, acct, started); e != nil {
			t.Fatalf("insert: %v", e)
		}
	}
	insert(1, recent)
	insert(1, old)
	insert(2, alsoRecent)
	insert(3, old)

	count, err := countUniquePlayers24h(context.Background(), db, now)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Errorf("unique 24h = %d, want 2 (accounts 1 and 2)", count)
	}
}

func TestCountUniquePlayers24h_NilDB(t *testing.T) {
	count, err := countUniquePlayers24h(context.Background(), nil, time.Now())
	if err != nil {
		t.Fatalf("nil db should be a graceful zero, got err: %v", err)
	}
	if count != 0 {
		t.Errorf("nil db count = %d, want 0", count)
	}
}

// ── config helpers ────────────────────────────────────────────────────────────

func TestDiscordStatusEnabled(t *testing.T) {
	tr, fa := true, false
	tests := []struct {
		name string
		cfg  appConfig
		want bool
	}{
		{"nil pointer defaults off", appConfig{}, false},
		{"explicit true", appConfig{DiscordStatusEnabled: &tr}, true},
		{"explicit false", appConfig{DiscordStatusEnabled: &fa}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := discordStatusEnabled(tt.cfg); got != tt.want {
				t.Errorf("discordStatusEnabled = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDiscordStatusInterval(t *testing.T) {
	tests := []struct {
		name string
		secs int
		want time.Duration
	}{
		{"zero defaults to 60s", 0, 60 * time.Second},
		{"below minimum is clamped", 5, statusMinInterval},
		{"valid value preserved", 120, 120 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := discordStatusInterval(appConfig{DiscordStatusIntervalSeconds: tt.secs}); got != tt.want {
				t.Errorf("discordStatusInterval(%d) = %v, want %v", tt.secs, got, tt.want)
			}
		})
	}
}

// ── loop lifecycle ────────────────────────────────────────────────────────────

func TestRunStatusLoop_StopsOnContextCancel(t *testing.T) {
	ticks := make(chan struct{}, 4)
	deps := statusLoopDeps{
		interval: time.Millisecond,
		tick: func(_ context.Context) {
			select {
			case ticks <- struct{}{}:
			default:
			}
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runStatusLoop(ctx, deps)
		close(done)
	}()

	// Wait for at least one tick to confirm the loop is running.
	select {
	case <-ticks:
	case <-time.After(time.Second):
		t.Fatal("loop did not tick")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("loop did not stop on context cancel")
	}
}

func TestApplyDiscordStatusLoop_TogglesCorrectly(t *testing.T) {
	// Disabled config must not start a loop.
	stopDiscordStatusLoop()
	applyDiscordStatusLoop(appConfig{})
	if statusLoopRunning() {
		t.Error("loop should not be running when disabled")
	}

	// Enabled but no channel → still no loop.
	tr := true
	applyDiscordStatusLoop(appConfig{DiscordStatusEnabled: &tr})
	if statusLoopRunning() {
		t.Error("loop should not run without a channel id")
	}

	// Enabled with channel → loop runs.
	applyDiscordStatusLoop(appConfig{DiscordStatusEnabled: &tr, DiscordStatusChannelID: "chan-1"})
	if !statusLoopRunning() {
		t.Error("loop should be running when enabled with a channel")
	}

	// Disabling again stops it.
	applyDiscordStatusLoop(appConfig{})
	if statusLoopRunning() {
		t.Error("loop should stop when toggled off")
	}
}

// ── server-state derivation ───────────────────────────────────────────────────

func TestDeriveServerState(t *testing.T) {
	tests := []struct {
		name   string
		status *BattlegroupStatus
		err    error
		want   serverState
	}{
		{"nil status is offline", nil, nil, serverStateOffline},
		{"error is offline", nil, errors.New("x"), serverStateOffline},
		{"no servers is offline", &BattlegroupStatus{Servers: nil}, nil, serverStateOffline},
		{
			"running server is online",
			&BattlegroupStatus{Servers: []ServerRow{{Map: "A", Phase: "Running", Ready: true}}},
			nil,
			serverStateOnline,
		},
		{
			"not-ready server is booting",
			&BattlegroupStatus{Servers: []ServerRow{{Map: "A", Phase: "Starting", Ready: false}}},
			nil,
			serverStateBooting,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriveServerState(tt.status, tt.err); got != tt.want {
				t.Errorf("deriveServerState = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAggregateMapCounts(t *testing.T) {
	servers := []ServerRow{
		{Map: "Hagga Basin", Players: 3},
		{Map: "Hagga Basin", Players: 2},
		{Map: "Deep Desert", Players: 1},
		{Map: "", Players: 4}, // unknown map name
	}
	got := aggregateMapCounts(servers)

	want := map[string]int{"Hagga Basin": 5, "Deep Desert": 1, "Unknown": 4}
	if len(got) != len(want) {
		t.Fatalf("got %d maps, want %d: %+v", len(got), len(want), got)
	}
	for _, mc := range got {
		if want[mc.Map] != mc.Players {
			t.Errorf("map %q = %d, want %d", mc.Map, mc.Players, want[mc.Map])
		}
	}
}
