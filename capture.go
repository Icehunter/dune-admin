package main

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"
	"unicode/utf8"

	jwt "github.com/golang-jwt/jwt/v5"
	amqp "github.com/rabbitmq/amqp091-go"
)

// ── JWT generation ────────────────────────────────────────────────────────────

// buildCaptureJWT parses an existing ServiceAuthToken and re-signs it with a
// fresh expiry. Shared across all ControlPlane implementations.
func buildCaptureJWT(existingToken string) (hostID, token string, err error) {
	parts := strings.Split(existingToken, ".")
	if len(parts) != 3 {
		return "", "", fmt.Errorf("malformed JWT")
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", fmt.Errorf("decode JWT payload: %w", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return "", "", fmt.Errorf("parse JWT payload: %w", err)
	}

	hostID = fmt.Sprintf("%v", claims["HostId"])
	serviceAuthKey := fmt.Sprintf("%v", claims["ServiceAuthKey"])
	fmt.Printf("[capture] HostId=%s ServiceHostType=%v\n", hostID, claims["ServiceHostType"])

	secret := "wus017CIPIkSB6+MhjvIAhWF+a+kVj+nW1AMb1mN1LfkUmcClqlmKeL69OT8BYuUA+Y4Vv44aUji4JBLeFfhxQ=="
	keyBytes, err := base64.StdEncoding.DecodeString(secret)
	if err != nil {
		keyBytes, err = base64.RawStdEncoding.DecodeString(secret)
		if err != nil {
			return "", "", fmt.Errorf("decode signing secret: %w", err)
		}
	}

	now := time.Now()
	newClaims := jwt.MapClaims{
		"HostId":          hostID,
		"TokenIndex":      "1",
		"ServiceAuthKey":  serviceAuthKey,
		"ServiceHostType": claims["ServiceHostType"],
		"nbf":             now.Unix(),
		"iat":             now.Unix(),
		"exp":             now.Add(365 * 24 * time.Hour).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, newClaims)
	token, err = tok.SignedString(keyBytes)
	if err != nil {
		return "", "", fmt.Errorf("sign JWT: %w", err)
	}
	fmt.Printf("[capture] generated JWT (%d bytes)\n", len(token))
	return hostID, token, nil
}

// dialAMQP connects to an AMQP broker at addr. TCP is routed through the
// global executor so it works for both direct and SSH-tunnelled connections.
func dialAMQP(addr, user, pass string, useTLS bool) (*amqp.Connection, error) {
	cfg := amqp.Config{
		SASL: []amqp.Authentication{
			&amqp.PlainAuth{Username: user, Password: pass},
		},
		Vhost:     "/",
		Locale:    "en_US",
		Heartbeat: 10 * time.Second,
		Dial: func(_, _ string) (net.Conn, error) {
			if globalExecutor != nil {
				return globalExecutor.Dial("tcp", addr)
			}
			// Fallback during transition before globalExecutor is set.
			if globalSSH != nil {
				return globalSSH.Dial("tcp", addr)
			}
			return net.Dial("tcp", addr)
		},
	}
	if useTLS {
		cfg.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // #nosec G402 -- internal RabbitMQ, self-signed cert
		return amqp.DialConfig("amqps://"+addr+"/", cfg)
	}
	return amqp.DialConfig("amqp://"+addr+"/", cfg)
}

// ── Capture entry point ───────────────────────────────────────────────────────

func runCapture() {
	ctx := context.Background()

	if globalControl != nil && globalExecutor != nil {
		globalControl.EnsureCaptureUser(ctx, globalExecutor)
		// AMP can restart the broker container at any time, which clears the
		// in-memory user list. Re-apply on a 15s tick so capture self-heals.
		if amp, ok := globalControl.(*ampControl); ok {
			amp.startEnsureCaptureUserLoop(globalExecutor)
		}
	}

	fmt.Println("=== Dune Admin — RabbitMQ Message Capture ===")
	fmt.Println("Press Ctrl-C to stop.")
	fmt.Println()

	// Get valid JWT credentials via the control plane.
	var hostID, token string
	if globalControl != nil && globalExecutor != nil {
		var err error
		hostID, token, err = globalControl.CaptureJWT(ctx, globalExecutor)
		if err != nil {
			fmt.Printf("[capture] JWT error: %v\n", err)
			fmt.Println("[capture] Falling back to dune_cap user (may not work)")
			hostID = capUser
			token = capPass
		}
	} else {
		hostID = capUser
		token = capPass
	}

	// Discover all exchanges on both brokers via the control plane.
	var adminBindings, gameBindings []binding
	if globalControl != nil && globalExecutor != nil {
		adminBindings, _ = globalControl.ListExchanges(ctx, globalExecutor, "mq-admin")
		gameBindings, _ = globalControl.ListExchanges(ctx, globalExecutor, "mq-game")
	}
	fmt.Printf("[capture] mq-admin: %d exchanges\n", len(adminBindings))
	fmt.Printf("[capture] mq-game:  %d exchanges\n", len(gameBindings))

	done := make(chan struct{}, 2)

	// Refresh auth backends every 15s — the cache TTL appears to be very short.
	go func() {
		for {
			time.Sleep(15 * time.Second)
			if globalControl != nil && globalExecutor != nil {
				globalControl.EnsureCaptureUser(context.Background(), globalExecutor)
			}
		}
	}()

	adminAddr := brokerAdminAddr
	if adminAddr == "" {
		adminAddr = "10.43.189.193:5672" // legacy K8s fallback
	}
	gameAddr := brokerGameAddr
	if gameAddr == "" {
		gameAddr = "10.43.48.246:5672" // legacy K8s fallback
	}
	gameTLS := brokerTLS || gameAddr == "10.43.48.246:5672"

	go func() {
		defer func() { done <- struct{}{} }()
		if err := captureBroker("mq-admin", adminAddr, false, hostID, token, adminBindings); err != nil {
			fmt.Printf("[WARN] mq-admin: %v\n\n", err)
		}
	}()

	go func() {
		defer func() { done <- struct{}{} }()
		if err := captureBroker("mq-game", gameAddr, gameTLS, hostID, token, gameBindings); err != nil {
			fmt.Printf("[WARN] mq-game: %v\n\n", err)
		}
	}()

	<-done
	<-done
}

// ── Per-broker capture ────────────────────────────────────────────────────────

type binding struct {
	exchange string
	key      string
}

const (
	capUser = "dune_cap"
	capPass = "DuneCap2026!"
)

func captureBroker(name, addr string, useTLS bool, user, pass string, bindings []binding) error {
	attempts := []struct{ u, p string }{
		{capUser, capPass},
		{user, pass},
		{pass, user},
	}

	for {

		var conn *amqp.Connection
		var connErr error
		for _, a := range attempts {
			conn, connErr = dialAMQP(addr, a.u, a.p, useTLS)
			if connErr == nil {
				fmt.Printf("[%s] connected (user=%s)\n", name, a.u)
				break
			}
		}
		if connErr != nil {
			return fmt.Errorf("connect (tried %d credential sets): %w", len(attempts), connErr)
		}

		func() {
			defer conn.Close()

			ch, err := conn.Channel()
			if err != nil {
				fmt.Printf("[%s] channel error: %v — reconnecting\n", name, err)
				return
			}
			defer ch.Close()

			q, err := ch.QueueDeclare("admin_capture_"+name, false, true, false, false, nil)
			if err != nil {
				fmt.Printf("[%s] queue error: %v — reconnecting\n", name, err)
				return
			}

			for _, b := range bindings {
				if err := ch.QueueBind(q.Name, b.key, b.exchange, false, nil); err != nil {
					fmt.Printf("[%s] bind %s: %v (skipping)\n", name, b.exchange, err)
					continue
				}
				fmt.Printf("[%s] ← %s (routing_key=%s)\n", name, b.exchange, b.key)
			}
			fmt.Printf("[%s] listening...\n\n", name)

			msgs, err := ch.Consume(q.Name, "", true, false, false, false, nil)
			if err != nil {
				fmt.Printf("[%s] consume error: %v — reconnecting\n", name, err)
				return
			}

			for msg := range msgs {
				printMessage(name, msg)
			}
			fmt.Printf("[%s] channel closed — reconnecting\n\n", name)
		}()

		time.Sleep(2 * time.Second)
	}
}

// ── Message printer ───────────────────────────────────────────────────────────

func printMessage(broker string, msg amqp.Delivery) {
	ts := time.Now().Format("15:04:05.000")
	body := msg.Body

	fmt.Printf("╔══ [%s] %s ═══════════════════════════════\n", broker, ts)
	fmt.Printf("║  Exchange:   %s\n", msg.Exchange)
	fmt.Printf("║  RoutingKey: %s\n", msg.RoutingKey)
	if msg.ContentType != "" {
		fmt.Printf("║  Type:       %s\n", msg.ContentType)
	}
	for k, v := range msg.Headers {
		fmt.Printf("║  Header[%s]: %v\n", k, v)
	}

	if isJSON(body) {
		var pretty any
		if err := json.Unmarshal(body, &pretty); err == nil {
			indented, _ := json.MarshalIndent(pretty, "║    ", "  ")
			fmt.Printf("║  Body:\n║    %s\n", indented)
		} else {
			fmt.Printf("║  Body: %s\n", body)
		}
	} else if utf8.Valid(body) && len(body) > 0 {
		fmt.Printf("║  Body (text): %s\n", body)
	} else if len(body) > 0 {
		fmt.Printf("║  Body (%d bytes hex): %x\n", len(body), body)
	} else {
		fmt.Printf("║  Body: (empty)\n")
	}
	fmt.Println("╚════════════════════════════════════════════")
	fmt.Println()
}

func isJSON(b []byte) bool {
	return len(b) > 0 && (b[0] == '{' || b[0] == '[')
}
