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

// ── server context fakes ──────────────────────────────────────────────────────

// statusFakeControl embeds stubControlPlane and scripts GetStatus for
// discord-status tests. stubControlPlane is defined in
// handlers_server_settings_ini_dir_test.go.
type statusFakeControl struct {
	stubControlPlane
	statusResult *BattlegroupStatus
	statusErr    error
}

func (f *statusFakeControl) GetStatus(_ context.Context, _ Executor) (*BattlegroupStatus, error) {
	return f.statusResult, f.statusErr
}

// ── applyControlStatus ────────────────────────────────────────────────────────

func TestApplyControlStatus_NilControl(t *testing.T) {
	t.Parallel()
	sc := &ServerContext{} // Control is nil
	data := statusEmbedData{State: serverStateOffline}
	applyControlStatus(context.Background(), sc, &data)
	if data.State != serverStateOffline {
		t.Errorf("state = %v, want offline (nil control must be a no-op)", data.State)
	}
}

func TestApplyControlStatus_UsesServerContextControl(t *testing.T) {
	t.Parallel()
	ctrl := &statusFakeControl{
		statusResult: &BattlegroupStatus{
			Servers: []ServerRow{
				{Map: "Hagga Basin", Phase: "Running", Ready: true, Players: 3},
			},
		},
	}
	sc := &ServerContext{Control: ctrl}
	data := statusEmbedData{State: serverStateOffline}
	applyControlStatus(context.Background(), sc, &data)
	if data.State != serverStateOnline {
		t.Errorf("state = %v, want online", data.State)
	}
	if data.CurrentOnline != 3 {
		t.Errorf("CurrentOnline = %d, want 3", data.CurrentOnline)
	}
	if len(data.Maps) != 1 || data.Maps[0].Map != "Hagga Basin" {
		t.Errorf("Maps = %+v, want [{Hagga Basin 3}]", data.Maps)
	}
}

func TestApplyControlStatus_ControlError(t *testing.T) {
	t.Parallel()
	ctrl := &statusFakeControl{statusErr: errors.New("control plane error")}
	sc := &ServerContext{Control: ctrl}
	data := statusEmbedData{State: serverStateOffline}
	applyControlStatus(context.Background(), sc, &data)
	// error → offline, no panic
	if data.State != serverStateOffline {
		t.Errorf("state = %v, want offline on error", data.State)
	}
}

// ── applyDBStats ──────────────────────────────────────────────────────────────

func TestApplyDBStats_NilDB(t *testing.T) {
	t.Parallel()
	sc := &ServerContext{} // DB is nil
	data := statusEmbedData{TotalPlayers: 99}
	applyDBStats(context.Background(), sc, &data)
	if data.TotalPlayers != 99 {
		t.Errorf("TotalPlayers changed, want 99 got %d", data.TotalPlayers)
	}
}

// ── collectStatusData ─────────────────────────────────────────────────────────

func TestCollectStatusData_NilServerContext(t *testing.T) {
	t.Parallel()
	data := collectStatusData(context.Background(), nil, nil)
	if data.State != serverStateOffline {
		t.Errorf("state = %v, want offline for nil ServerContext", data.State)
	}
}

func TestCollectStatusData_OnlineServerContext(t *testing.T) {
	t.Parallel()
	ctrl := &statusFakeControl{
		statusResult: &BattlegroupStatus{
			Servers: []ServerRow{
				{Map: "Deep Desert", Phase: "Running", Ready: true, Players: 7},
			},
		},
	}
	sdb, err := openUnifiedStore(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = sdb.Close() }()

	sc := &ServerContext{Control: ctrl, StoreScope: defaultServerID}
	data := collectStatusData(context.Background(), sc, sdb)
	if data.State != serverStateOnline {
		t.Errorf("state = %v, want online", data.State)
	}
	if data.CurrentOnline != 7 {
		t.Errorf("CurrentOnline = %d, want 7", data.CurrentOnline)
	}
}

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
	if _, err := db.Exec(`INSERT INTO servers (id, name, position) VALUES (?, 'srv', 0)`, defaultServerID); err != nil {
		t.Fatalf("insert server: %v", err)
	}

	store := newSqliteStatusStore(db, defaultServerID)

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
	if _, err := db.Exec(`INSERT INTO servers (id, name, position) VALUES (?, 'srv', 0)`, defaultServerID); err != nil {
		t.Fatalf("insert server: %v", err)
	}

	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-2 * time.Hour).Format(time.RFC3339)
	alsoRecent := now.Add(-23 * time.Hour).Format(time.RFC3339)
	old := now.Add(-48 * time.Hour).Format(time.RFC3339)

	// account 1 appears twice (recent + old), account 2 once recent, account 3 old only.
	insert := func(acct int64, started string) {
		if _, e := db.Exec(`INSERT INTO play_sessions(server_id, account_id, started_at) VALUES(?, ?, ?)`, defaultServerID, acct, started); e != nil {
			t.Fatalf("insert: %v", e)
		}
	}
	insert(1, recent)
	insert(1, old)
	insert(2, alsoRecent)
	insert(3, old)

	count, err := countUniquePlayers24h(context.Background(), db, defaultServerID, now)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Errorf("unique 24h = %d, want 2 (accounts 1 and 2)", count)
	}
}

func TestCountUniquePlayers24h_NilDB(t *testing.T) {
	count, err := countUniquePlayers24h(context.Background(), nil, defaultServerID, time.Now())
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

func TestApplyDiscordStatusLoops_TogglesCorrectly(t *testing.T) {
	// Swap in an in-memory guilds store for the duration of the test.
	prevStore := globalDiscordGuildsStore
	t.Cleanup(func() {
		globalDiscordGuildsStore = prevStore
		stopDiscordStatusLoop()
	})

	db := openMemUnifiedStoreFK(t)
	sid := int(insertTestServer(t, db, "S"))
	store := newDiscordGuildsStore(db)
	globalDiscordGuildsStore = store
	if err := store.upsertGuild(discordGuild{GuildID: "g1"}); err != nil {
		t.Fatalf("upsert guild: %v", err)
	}

	// No links → no loops.
	stopDiscordStatusLoop()
	applyDiscordStatusLoops()
	if statusLoopRunning() {
		t.Error("loop should not run with no links")
	}

	upsert := func(link discordServerLink) {
		if err := store.upsertServerLink(link); err != nil {
			t.Fatalf("upsert server link: %v", err)
		}
	}

	// Status disabled → no loop.
	upsert(discordServerLink{ServerID: sid, GuildID: "g1", StatusEnabled: false, StatusChannelID: "chan-1"})
	applyDiscordStatusLoops()
	if statusLoopRunning() {
		t.Error("loop should not run when status disabled")
	}

	// Enabled but no channel → no loop.
	upsert(discordServerLink{ServerID: sid, GuildID: "g1", StatusEnabled: true, StatusChannelID: ""})
	applyDiscordStatusLoops()
	if statusLoopRunning() {
		t.Error("loop should not run without a channel id")
	}

	// Enabled with channel → loop runs.
	upsert(discordServerLink{ServerID: sid, GuildID: "g1", StatusEnabled: true, StatusChannelID: "chan-1"})
	applyDiscordStatusLoops()
	if !statusLoopRunning() {
		t.Error("loop should run when enabled with a channel")
	}

	// Removing the link stops it.
	if err := store.deleteServerLink(sid); err != nil {
		t.Fatalf("delete server link: %v", err)
	}
	applyDiscordStatusLoops()
	if statusLoopRunning() {
		t.Error("loop should stop when the only enabled link is removed")
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
	t.Parallel()

	check := func(t *testing.T, got []mapPlayerCount, want map[string]int) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("got %d maps, want %d: %+v", len(got), len(want), got)
		}
		for _, mc := range got {
			v, ok := want[mc.Map]
			if !ok {
				t.Errorf("unexpected label %q", mc.Map)
				continue
			}
			if mc.Players != v {
				t.Errorf("label %q players = %d, want %d", mc.Map, mc.Players, v)
			}
		}
	}

	t.Run("two partitions same map, no director label", func(t *testing.T) {
		t.Parallel()
		servers := []ServerRow{
			{Map: "Survival_1", Partition: 1, Players: 3},
			{Map: "Survival_2", Partition: 2, Players: 2},
			{Map: "DeepDesert", Players: 1},
			{Map: "", Players: 4},
		}
		check(t, aggregateMapCounts(servers), map[string]int{
			"Hagga Basin #1": 3,
			"Hagga Basin #2": 2,
			"Deep Desert":    1,
			"Unknown":        4,
		})
	})

	t.Run("region name preferred over director label", func(t *testing.T) {
		t.Parallel()
		servers := []ServerRow{
			// The director codename ("Abbir") is ignored in favour of the region.
			{Map: "Survival_1", Sietch: "Abbir", Partition: 1, Players: 12},
			{Map: "DeepDesert", Sietch: "Deep Desert", Partition: 2, Players: 3},
			// Unknown map → fall back to the director Sietch label.
			{Map: "", Sietch: "Custom Arena", Partition: 5, Players: 2},
		}
		check(t, aggregateMapCounts(servers), map[string]int{
			"Hagga Basin":  12,
			"Deep Desert":  3,
			"Custom Arena": 2,
		})
	})

	t.Run("survival home instance uses server display name", func(t *testing.T) {
		t.Parallel()
		// DisplayName is server-wide; only the Survival/Hagga Basin home instance
		// uses it. Other maps keep their region names.
		servers := []ServerRow{
			{Map: "Survival_1", Sietch: "Abbir", DisplayName: "Sietch Umbu", Partition: 1, Players: 2},
			{Map: "Overmap", Sietch: "Overland", DisplayName: "Sietch Umbu", Partition: 2, Players: 1},
			{Map: "DeepDesert_1", DisplayName: "Sietch Umbu", Partition: 8, Players: 0},
		}
		check(t, aggregateMapCounts(servers), map[string]int{
			"Sietch Umbu": 2,
			"Overland":    1,
			"Deep Desert": 0,
		})
	})

	t.Run("single partition no director label, no suffix", func(t *testing.T) {
		t.Parallel()
		servers := []ServerRow{
			{Map: "Survival_1", Partition: 1, Players: 5},
		}
		got := aggregateMapCounts(servers)
		if len(got) != 1 {
			t.Fatalf("got %d maps, want 1: %+v", len(got), got)
		}
		if got[0].Map != "Hagga Basin" {
			t.Errorf("label = %q, want %q", got[0].Map, "Hagga Basin")
		}
		if got[0].Players != 5 {
			t.Errorf("players = %d, want 5", got[0].Players)
		}
	})

	t.Run("unknown map name buckets as Unknown", func(t *testing.T) {
		t.Parallel()
		servers := []ServerRow{
			{Map: "", Players: 4},
		}
		got := aggregateMapCounts(servers)
		if len(got) != 1 || got[0].Map != "Unknown" || got[0].Players != 4 {
			t.Errorf("got %+v, want [{Unknown 4}]", got)
		}
	})
}
