package main

import (
	"context"
	"errors"
	"sort"
	"testing"
)

func TestPrettyRegionName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in, want string
	}{
		{"HaggaBasin", "Hagga Basin"},
		{"TheShield", "The Shield"},
		{"Deepdesert", "Deep Desert"},
		{"VermillionWells", "Vermillion Wells"},
		{"", ""},
		{"OVERMAP", "Overland"},
		{"ARRAKEEN", "ARRAKEEN"},
		{"Map_HaggaBasin", "Hagga Basin"}, // strips a leading Map_ prefix
		{"Survival_1", "Hagga Basin"},     // aliases and trims numbers
		{"DeepDesert_0", "Deep Desert"},   // trims numbers
	}
	for _, tt := range tests {
		if got := prettyRegionName(tt.in); got != tt.want {
			t.Errorf("prettyRegionName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRenderRegionAnnouncement(t *testing.T) {
	t.Parallel()
	got := renderRegionAnnouncement("{player} entered {region}!", "Paul", "HaggaBasin")
	want := "Paul entered Hagga Basin!"
	if got != want {
		t.Errorf("render = %q, want %q", got, want)
	}
	// Blank player falls back to the default name so the message is never malformed.
	got = renderRegionAnnouncement("{player} left", "", "TheShield")
	if want = regionDefaultPlayerName + " left"; got != want {
		t.Errorf("render blank player = %q, want %q", got, want)
	}
}

func TestRegionAnnouncementsFor_JoinAndLeave(t *testing.T) {
	t.Parallel()
	joins := []welcomeAccount{{AccountID: 1, CharacterName: "Paul", Region: "HaggaBasin"}}
	leaves := []welcomeAccount{{AccountID: 2, CharacterName: "Chani", Region: "TheShield"}}
	cfg := regionBroadcastConfig{
		joinEnabled:   true,
		leaveEnabled:  true,
		joinTemplate:  "{player} arrived in {region}",
		leaveTemplate: "{player} left {region}",
		sourcePlayer:  "",
	}
	anns := regionAnnouncementsFor(joins, leaves, cfg)
	if len(anns) != 2 {
		t.Fatalf("want 2 announcements, got %d (%+v)", len(anns), anns)
	}
	byRegion := map[string]regionAnnouncement{}
	for _, a := range anns {
		byRegion[a.region] = a
	}
	if got := byRegion["HaggaBasin"].text; got != "Paul arrived in Hagga Basin" {
		t.Errorf("join text = %q", got)
	}
	if got := byRegion["TheShield"].text; got != "Chani left The Shield" {
		t.Errorf("leave text = %q", got)
	}
}

func TestRegionAnnouncementsFor_Disabled(t *testing.T) {
	t.Parallel()
	joins := []welcomeAccount{{AccountID: 1, CharacterName: "Paul", Region: "HaggaBasin"}}
	leaves := []welcomeAccount{{AccountID: 2, CharacterName: "Chani", Region: "TheShield"}}

	// Both off → nothing.
	if anns := regionAnnouncementsFor(joins, leaves, regionBroadcastConfig{
		joinTemplate: "j", leaveTemplate: "l",
	}); len(anns) != 0 {
		t.Fatalf("both disabled: want 0, got %+v", anns)
	}

	// Join on, leave off → only the join.
	anns := regionAnnouncementsFor(joins, leaves, regionBroadcastConfig{
		joinEnabled: true, joinTemplate: "{player} in {region}", leaveTemplate: "l",
	})
	if len(anns) != 1 || anns[0].region != "HaggaBasin" {
		t.Fatalf("join-only: want 1 join announcement, got %+v", anns)
	}
}

func TestRegionAnnouncementsFor_BlankTemplate(t *testing.T) {
	t.Parallel()
	joins := []welcomeAccount{{AccountID: 1, CharacterName: "Paul", Region: "HaggaBasin"}}
	// Enabled but blank template → no announcement (never whisper an empty body).
	if anns := regionAnnouncementsFor(joins, nil, regionBroadcastConfig{
		joinEnabled: true, joinTemplate: "   ",
	}); len(anns) != 0 {
		t.Fatalf("blank template: want 0, got %+v", anns)
	}
}

func TestRegionAnnouncementsFor_NoRegionSkipped(t *testing.T) {
	t.Parallel()
	// A join with an unknown region cannot target anyone — skip it.
	joins := []welcomeAccount{{AccountID: 1, CharacterName: "Paul", Region: ""}}
	if anns := regionAnnouncementsFor(joins, nil, regionBroadcastConfig{
		joinEnabled: true, joinTemplate: "{player} in {region}",
	}); len(anns) != 0 {
		t.Fatalf("blank region: want 0, got %+v", anns)
	}
}

// fakeChatSender records every whisper so tests can assert who was messaged.
type fakeChatSender struct {
	sent []struct {
		accountID int64
		message   string
	}
	err error
}

func (f *fakeChatSender) send(_ context.Context, accountID int64, _ string, message string) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, struct {
		accountID int64
		message   string
	}{accountID, message})
	return nil
}

func TestRunRegionBroadcastOnJoinLeave_WhispersRegionPlayers(t *testing.T) {
	t.Parallel()
	// Online set: two players in HaggaBasin, one in TheShield.
	online := []welcomeAccount{
		{AccountID: 1, CharacterName: "Paul", Region: "HaggaBasin"},
		{AccountID: 2, CharacterName: "Jessica", Region: "HaggaBasin"},
		{AccountID: 3, CharacterName: "Stilgar", Region: "TheShield"},
	}
	// Paul (acct 1) just joined HaggaBasin.
	joins := []welcomeAccount{{AccountID: 1, CharacterName: "Paul", Region: "HaggaBasin"}}
	cfg := regionBroadcastConfig{joinEnabled: true, joinTemplate: "{player} entered {region}"}

	sender := &fakeChatSender{}
	runRegionBroadcastOnJoinLeave(context.Background(), joins, nil, online, cfg, sender.send)

	// Both HaggaBasin players (1 and 2) get the notice; the TheShield player (3)
	// must NOT, since the event is region-scoped.
	var got []int64
	for _, s := range sender.sent {
		got = append(got, s.accountID)
		if s.message != "Paul entered Hagga Basin" {
			t.Errorf("unexpected message %q", s.message)
		}
	}
	sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("want whispers to accts [1 2], got %v", got)
	}
}

func TestRunRegionBroadcastOnJoinLeave_LeaveTargetsRemainingPlayers(t *testing.T) {
	t.Parallel()
	// After Chani left TheShield, only Stilgar remains there.
	online := []welcomeAccount{
		{AccountID: 3, CharacterName: "Stilgar", Region: "TheShield"},
	}
	leaves := []welcomeAccount{{AccountID: 2, CharacterName: "Chani", Region: "TheShield"}}
	cfg := regionBroadcastConfig{leaveEnabled: true, leaveTemplate: "{player} left {region}"}

	sender := &fakeChatSender{}
	runRegionBroadcastOnJoinLeave(context.Background(), nil, leaves, online, cfg, sender.send)

	if len(sender.sent) != 1 || sender.sent[0].accountID != 3 {
		t.Fatalf("want 1 whisper to acct 3, got %+v", sender.sent)
	}
	if sender.sent[0].message != "Chani left The Shield" {
		t.Errorf("message = %q", sender.sent[0].message)
	}
}

func TestRunRegionBroadcastOnJoinLeave_SenderErrorDoesNotPanic(t *testing.T) {
	t.Parallel()
	online := []welcomeAccount{{AccountID: 1, Region: "HaggaBasin"}}
	joins := []welcomeAccount{{AccountID: 1, CharacterName: "Paul", Region: "HaggaBasin"}}
	cfg := regionBroadcastConfig{joinEnabled: true, joinTemplate: "{player} in {region}"}
	sender := &fakeChatSender{err: errors.New("broker down")}
	// Must not panic even when every send fails.
	runRegionBroadcastOnJoinLeave(context.Background(), joins, nil, online, cfg, sender.send)
}

// ── map chat broadcast tests ─────────────────────────────────────────────────

// fakeMapSender records map chat publishes: one per announcement, keyed on region.
type fakeMapSender struct {
	sent []struct {
		region  string
		message string
	}
	err error
}

func (f *fakeMapSender) send(_ context.Context, region, _ string, message string) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, struct {
		region  string
		message string
	}{region, message})
	return nil
}

// TestRunMapChatBroadcastOnJoinLeave_PublishesOncePerEvent verifies that map chat
// sends exactly one publish per join/leave event (not one per player), which is
// the key efficiency difference from the whisper path.
func TestRunMapChatBroadcastOnJoinLeave_PublishesOncePerEvent(t *testing.T) {
	t.Parallel()
	joins := []welcomeAccount{{AccountID: 1, CharacterName: "Paul", Region: "HaggaBasin"}}
	leaves := []welcomeAccount{{AccountID: 2, CharacterName: "Chani", Region: "Arrakeen"}}
	cfg := regionBroadcastConfig{
		joinEnabled:   true,
		leaveEnabled:  true,
		joinTemplate:  "{player} arrived in {region}",
		leaveTemplate: "{player} left {region}",
	}
	sender := &fakeMapSender{}
	runMapChatBroadcastOnJoinLeave(context.Background(), joins, leaves, cfg, sender.send)

	if len(sender.sent) != 2 {
		t.Fatalf("want 2 publishes, got %d", len(sender.sent))
	}
	byRegion := map[string]string{}
	for _, s := range sender.sent {
		byRegion[s.region] = s.message
	}
	if got := byRegion["HaggaBasin"]; got != "Paul arrived in Hagga Basin" {
		t.Errorf("join text = %q", got)
	}
	if got := byRegion["Arrakeen"]; got != "Chani left Arrakeen" {
		t.Errorf("leave text = %q", got)
	}
}

func TestRunMapChatBroadcastOnJoinLeave_NoAnnouncementsNoSend(t *testing.T) {
	t.Parallel()
	joins := []welcomeAccount{{AccountID: 1, CharacterName: "Paul", Region: "HaggaBasin"}}
	cfg := regionBroadcastConfig{joinEnabled: false, joinTemplate: "{player} in {region}"}
	sender := &fakeMapSender{}
	runMapChatBroadcastOnJoinLeave(context.Background(), joins, nil, cfg, sender.send)
	if len(sender.sent) != 0 {
		t.Fatalf("want 0 sends for disabled config, got %d", len(sender.sent))
	}
}

func TestRunMapChatBroadcastOnJoinLeave_SenderErrorDoesNotPanic(t *testing.T) {
	t.Parallel()
	joins := []welcomeAccount{{AccountID: 1, CharacterName: "Paul", Region: "HaggaBasin"}}
	cfg := regionBroadcastConfig{joinEnabled: true, joinTemplate: "{player} in {region}"}
	sender := &fakeMapSender{err: errors.New("broker down")}
	runMapChatBroadcastOnJoinLeave(context.Background(), joins, nil, cfg, sender.send)
	// must not panic
}
