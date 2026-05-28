package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// brokerCredentials returns the configured AMQP username and password.
// Both BROKER_USER and BROKER_PASS (or config equivalents) are required.
func brokerCredentials() (user, pass string, err error) {
	user = brokerUser
	pass = brokerPass
	if user == "" || pass == "" {
		return "", "", fmt.Errorf("broker credentials are required: set BROKER_USER and BROKER_PASS")
	}
	return user, pass, nil
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
			if globalSSH != nil {
				return globalSSH.Dial("tcp", addr)
			}
			return net.Dial("tcp", addr)
		},
	}
	if useTLS {
		cfg.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
		return amqp.DialConfig("amqps://"+addr+"/", cfg)
	}
	return amqp.DialConfig("amqp://"+addr+"/", cfg)
}
