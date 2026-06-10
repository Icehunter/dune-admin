package main

import "testing"

func TestRenderMOTD(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		template string
		acc      welcomeAccount
		want     string
	}{
		{"player placeholder", "Welcome, {player}!", welcomeAccount{CharacterName: "Paul"}, "Welcome, Paul!"},
		{"repeated placeholder", "{player}, hello {player}", welcomeAccount{CharacterName: "Chani"}, "Chani, hello Chani"},
		{"no placeholder", "Welcome to the server", welcomeAccount{CharacterName: "Paul"}, "Welcome to the server"},
		{"blank name falls back", "Hi {player}", welcomeAccount{CharacterName: ""}, "Hi " + motdDefaultPlayerName},
		{"whitespace name falls back", "Hi {player}", welcomeAccount{CharacterName: "   "}, "Hi " + motdDefaultPlayerName},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := renderMOTD(tt.template, tt.acc); got != tt.want {
				t.Fatalf("renderMOTD(%q, %+v) = %q, want %q", tt.template, tt.acc, got, tt.want)
			}
		})
	}
}

func TestMotdWhispersForJoins(t *testing.T) {
	t.Parallel()
	joins := []welcomeAccount{
		{AccountID: 10, CharacterName: "Paul"},
		{AccountID: 11, CharacterName: "Chani"},
	}

	t.Run("disabled yields nothing", func(t *testing.T) {
		t.Parallel()
		if got := motdWhispersForJoins(joins, false, "Welcome {player}", ""); got != nil {
			t.Fatalf("disabled: want nil, got %+v", got)
		}
	})
	t.Run("blank message yields nothing", func(t *testing.T) {
		t.Parallel()
		if got := motdWhispersForJoins(joins, true, "   ", ""); got != nil {
			t.Fatalf("blank message: want nil, got %+v", got)
		}
	})
	t.Run("one rendered whisper per join", func(t *testing.T) {
		t.Parallel()
		got := motdWhispersForJoins(joins, true, "Welcome {player}", "SRC")
		if len(got) != 2 {
			t.Fatalf("want 2 whispers, got %d (%+v)", len(got), got)
		}
		if got[0].accountID != 10 || got[0].message != "Welcome Paul" || got[0].sourcePlayer != "SRC" {
			t.Fatalf("whisper[0] = %+v", got[0])
		}
		if got[1].accountID != 11 || got[1].message != "Welcome Chani" {
			t.Fatalf("whisper[1] = %+v", got[1])
		}
	})
	t.Run("no joins yields empty", func(t *testing.T) {
		t.Parallel()
		if got := motdWhispersForJoins(nil, true, "hi", ""); len(got) != 0 {
			t.Fatalf("no joins: want empty, got %+v", got)
		}
	})
}
