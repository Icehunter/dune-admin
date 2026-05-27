package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
)

// ── mq-game publisher ─────────────────────────────────────────────────────────

var (
	mqGameConn *amqp.Connection
	mqGameCh   *amqp.Channel
	mqGameMu   sync.Mutex
)

func mqGameChannel() (*amqp.Channel, error) {
	mqGameMu.Lock()
	defer mqGameMu.Unlock()

	if mqGameCh != nil && !mqGameCh.IsClosed() {
		return mqGameCh, nil
	}
	if mqGameConn != nil && !mqGameConn.IsClosed() {
		_ = mqGameConn.Close()
	}

	addr := brokerGameAddr
	if addr == "" {
		// Legacy fallback for existing K8s configs that predate broker_game_addr.
		addr = "10.43.48.246:5672"
	}

	user, pass, err := brokerCredentials()
	if err != nil {
		return nil, err
	}
	conn, err := dialAMQP(addr, user, pass, brokerTLS || addr == "10.43.48.246:5672")
	if err != nil {
		return nil, fmt.Errorf("mq-game connect: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("mq-game channel: %w", err)
	}
	mqGameConn = conn
	mqGameCh = ch
	return ch, nil
}

// publishNotification sends a CourierNotification to the mq-game notifications
// exchange. routingKey controls which server queues receive it ("PlayerOnlineState",
// "#" for broadcast, etc.). keywords controls what the game client does with it.
func publishNotification(routingKey string, keywords []string, content string) error {
	ch, err := mqGameChannel()
	if err != nil {
		return err
	}

	// Inner payload (content field of the courier).
	inner, _ := json.Marshal(map[string]any{
		"RoutingInfo": map[string]any{"Keywords": keywords},
		"content":     content,
		"SenderId":    1,
	})

	// Outer CourierNotification envelope.
	outer, _ := json.Marshal(map[string]any{
		"Type":    "CourierNotification",
		"content": string(inner),
	})

	err = ch.Publish(
		"notifications", // exchange
		routingKey,      // routing key
		false,           // mandatory
		false,           // immediate
		amqp.Publishing{
			ContentType: "Content",
			Body:        outer,
		},
	)
	if err != nil {
		// Channel may have died — clear it so next call reconnects.
		mqGameMu.Lock()
		mqGameCh = nil
		mqGameMu.Unlock()
		return fmt.Errorf("publish: %w", err)
	}
	return nil
}

// ── HTTP handler ──────────────────────────────────────────────────────────────

// handleNotify publishes an in-game notification via mq-game.
//
// POST /api/v1/notify
//
//	{
//	  "routing_key": "PlayerOnlineState",  // optional, default "#"
//	  "keywords":    ["PlayerOnlineState"],  // optional, default ["AdminMessage"]
//	  "content":     "Hello World!"
//	}
func handleNotify(w http.ResponseWriter, r *http.Request) {
	if globalExecutor == nil {
		jsonErr(w, fmt.Errorf("not connected"), http.StatusServiceUnavailable)
		return
	}
	var req struct {
		RoutingKey string   `json:"routing_key"`
		Keywords   []string `json:"keywords"`
		Content    string   `json:"content"`
	}
	if err := decode(r, &req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	if req.Content == "" {
		jsonErr(w, fmt.Errorf("content required"), 400)
		return
	}
	if req.RoutingKey == "" {
		req.RoutingKey = "PlayerOnlineState"
	}
	if len(req.Keywords) == 0 {
		req.Keywords = []string{"PlayerOnlineState"}
	}

	if err := publishNotification(req.RoutingKey, req.Keywords, req.Content); err != nil {
		jsonErr(w, err, 500)
		return
	}
	jsonOK(w, map[string]string{"ok": "notification sent"})
}
