package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
)

// captureJWTSecretB64 is the baked-in base64 HMAC key used for CaptureJWT.
// It may be overridden at build time via:
//
//	-ldflags "-X main.captureJWTSecretB64=<base64-secret>"
//
// Optional runtime override remains available via BROKER_JWT_SECRET.
//
//nolint:gosec // intentional baked secret for AMP/FLS compatibility
var captureJWTSecretB64 = "wus017CIPIkSB6+MhjvIAhWF+a+kVj+nW1AMb1mN1LfkUmcClqlmKeL69OT8BYuUA+Y4Vv44aUji4JBLeFfhxQ=="

// buildCaptureJWT parses an existing ServiceAuthToken and re-signs it with a
// fresh expiry. Used by CaptureJWT in all ControlPlane implementations.
//
// The HMAC signing secret defaults to captureJWTSecretB64 (baked into the
// binary), with optional runtime override from BROKER_JWT_SECRET.
func buildCaptureJWT(existingToken string) (hostID, token string, err error) {
	parts := splitJWT(existingToken)
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

	secretB64 := captureJWTSecretB64
	if secretB64 == "" {
		// Per-server after the remodel — the global loadedConfig field is cleared.
		secretB64 = activeServerCfg().BrokerJWTSecret
	}
	if envSecret := os.Getenv("BROKER_JWT_SECRET"); envSecret != "" {
		secretB64 = envSecret
	}
	if secretB64 == "" {
		return "", "", fmt.Errorf("capture JWT signing secret is empty")
	}
	keyBytes, err := base64.StdEncoding.DecodeString(secretB64)
	if err != nil {
		keyBytes, err = base64.RawStdEncoding.DecodeString(secretB64)
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
	return hostID, token, nil
}

func splitJWT(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	return append(parts, s[start:])
}
