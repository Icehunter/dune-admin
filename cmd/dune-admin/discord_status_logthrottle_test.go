package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/bwmarrin/discordgo"
)

func restErr(code int) error {
	return &discordgo.RESTError{
		Message: &discordgo.APIErrorMessage{Code: code, Message: "x"},
	}
}

func TestIsMissingPermissionsErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"missing permissions 50013", restErr(discordgo.ErrCodeMissingPermissions), true},
		{"missing access 50001", restErr(discordgo.ErrCodeMissingAccess), true},
		{"wrapped missing permissions", fmt.Errorf("send status embed: %w", restErr(discordgo.ErrCodeMissingPermissions)), true},
		{"unknown message is not a permission error", restErr(discordgo.ErrCodeUnknownMessage), false},
		{"plain error", errors.New("boom"), false},
		{"nil", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMissingPermissionsErr(tt.err); got != tt.want {
				t.Fatalf("isMissingPermissionsErr = %v, want %v", got, tt.want)
			}
		})
	}
}

// A persistent failure must warn exactly once; recovery logs once; a reset
// (loop restart) re-arms the warning.
func TestNextStatusLogAction(t *testing.T) {
	const sid = 987654 // unique id so the package-global map isn't shared with other tests
	resetStatusFailState(sid)

	if a := nextStatusLogAction(sid, true); a != statusLogWarn {
		t.Fatalf("first failure: got %v, want warn", a)
	}
	if a := nextStatusLogAction(sid, true); a != statusLogSuppress {
		t.Fatalf("repeated failure: got %v, want suppress", a)
	}
	if a := nextStatusLogAction(sid, false); a != statusLogRecovered {
		t.Fatalf("recovery: got %v, want recovered", a)
	}
	if a := nextStatusLogAction(sid, false); a != statusLogSuppress {
		t.Fatalf("steady success: got %v, want suppress", a)
	}

	// A loop restart re-arms the warning for the next failure.
	resetStatusFailState(sid)
	if a := nextStatusLogAction(sid, true); a != statusLogWarn {
		t.Fatalf("post-reset failure: got %v, want warn", a)
	}
}
