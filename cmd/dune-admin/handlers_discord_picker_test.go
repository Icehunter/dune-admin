package main

import (
	"errors"
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestCmdListDiscordGuilds(t *testing.T) {
	tests := []struct {
		name    string
		raw     []*discordgo.UserGuild
		fetjErr error
		want    []discordGuildOption
		wantErr bool
	}{
		{
			name: "maps id and name",
			raw: []*discordgo.UserGuild{
				{ID: "111", Name: "Dune Alpha"},
				{ID: "222", Name: "Dune Beta"},
			},
			want: []discordGuildOption{
				{ID: "111", Name: "Dune Alpha"},
				{ID: "222", Name: "Dune Beta"},
			},
		},
		{
			name: "empty list yields empty slice",
			raw:  []*discordgo.UserGuild{},
			want: []discordGuildOption{},
		},
		{
			name:    "propagates fetch error",
			fetjErr: errors.New("rate limited"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := cmdListDiscordGuilds(func() ([]*discordgo.UserGuild, error) {
				return tt.raw, tt.fetjErr
			})
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d guilds, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("guild[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestCmdListDiscordChannels(t *testing.T) {
	tests := []struct {
		name     string
		raw      []*discordgo.Channel
		fetchErr error
		want     []discordChannelOption
		wantErr  bool
	}{
		{
			name: "keeps text and announcement channels, drops voice/category",
			raw: []*discordgo.Channel{
				{ID: "1", Name: "general", Type: discordgo.ChannelTypeGuildText},
				{ID: "2", Name: "Voice Chat", Type: discordgo.ChannelTypeGuildVoice},
				{ID: "3", Name: "announcements", Type: discordgo.ChannelTypeGuildNews},
				{ID: "4", Name: "Category", Type: discordgo.ChannelTypeGuildCategory},
			},
			want: []discordChannelOption{
				{ID: "1", Name: "general"},
				{ID: "3", Name: "announcements"},
			},
		},
		{
			name: "no postable channels yields empty slice",
			raw: []*discordgo.Channel{
				{ID: "9", Name: "Voice", Type: discordgo.ChannelTypeGuildVoice},
			},
			want: []discordChannelOption{},
		},
		{
			name:     "propagates fetch error",
			fetchErr: errors.New("missing access"),
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := cmdListDiscordChannels("g1", func(_ string) ([]*discordgo.Channel, error) {
				return tt.raw, tt.fetchErr
			})
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d channels, want %d (%+v)", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("channel[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
