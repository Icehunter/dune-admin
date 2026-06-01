package main

import (
	"strings"
	"testing"
)

// TestBuildCourierPublishExpr_InjectsUserIDAndType locks the two publish-layer
// fixes that the live-confirmed whisper recipe requires:
//
//   - AMQP user_id must be the SENDER's hex FLS id (so the game's player-info
//     lookup resolves a real identity). The previous code hardcoded <<"fls">>,
//     which the game silently drops.
//   - AMQP type must be the notification type NAME "text_chat", not the byte
//     string "12" the previous code sent.
//
// The expr publishes via rabbitmqctl eval, which builds the P_basic record
// directly and so may set user_id freely (AMQP clients cannot).
func TestBuildCourierPublishExpr_InjectsUserIDAndType(t *testing.T) {
	t.Parallel()

	expr := buildCourierPublishExpr("chat.whispers", "Tester#1234", "Qk9EWQ==", "msg-1", "text_chat", "GMHEXUSER")

	// type, user_id, app_id must occupy adjacent P_basic slots in that order.
	if !strings.Contains(expr, `<<"text_chat">>, <<"GMHEXUSER">>, <<"fls_backend">>`) {
		t.Fatalf("type/user_id/app_id slots wrong:\n%s", expr)
	}

	for _, want := range []string{
		`<<"chat.whispers">>`,          // exchange
		`<<"Tester#1234">>`,            // routing key
		`base64:decode(<<"Qk9EWQ==">>`, // body
		`<<"msg-1">>`,                  // message id
	} {
		if !strings.Contains(expr, want) {
			t.Fatalf("expr missing %q:\n%s", want, expr)
		}
	}

	// Regression: the old code hardcoded fls as the user_id.
	if strings.Contains(expr, `<<"fls">>, <<"fls_backend">>`) {
		t.Fatalf("user_id still hardcoded to fls:\n%s", expr)
	}
}
