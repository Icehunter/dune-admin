package main

import "regexp"

// redactRule masks a category of sensitive content. Order matters: token and
// path rules run before the host rule so their internal host-like substrings
// are masked under the more specific label.
type redactRule struct {
	re   *regexp.Regexp
	repl string
}

var redactRules = []redactRule{
	// bearer / authorization tokens
	{regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._\-]+`), "Bearer [redacted-token]"},
	// ServiceAuthToken even when space-separated from its (long) value
	{regexp.MustCompile(`(?i)serviceauthtoken["'\s:=]+[A-Za-z0-9._\-]{12,}`), "ServiceAuthToken [redacted-token]"},
	// credentials embedded in a URL / DSN userinfo: scheme://user:pass@host
	{regexp.MustCompile(`://[^/\s:@]+:[^/\s@]+@`), "://[redacted-creds]@"},
	// sensitive key/value: k=v, k: v, and JSON "k":"v" / "k": v forms
	{regexp.MustCompile(`(?i)"?\b(serviceauthtoken|api[_-]?key|access[_-]?token|refresh[_-]?token|token|key|password|passwd|secret)\b"?\s*[:=]\s*"?[^"\s,}]+`), "$1=[redacted-token]"},
	// numeric account / player / fls ids (k=v, k:v, JSON "k":v)
	{regexp.MustCompile(`(?i)"?\b(account_id|player_id|fls_id|owner_id)\b"?\s*[:=]\s*"?\d+`), "$1=[redacted-id]"},
	// home directory paths (mask the username segment and below)
	{regexp.MustCompile(`(?i)(/home/|/Users/|C:\\Users\\)[^\s"']+`), "[redacted-path]"},
	// IPv6 (compressed or full, optional brackets, optional :port). Runs before the
	// IPv4/user@host rules so bracketed [ipv6]:port and bare forms are caught first.
	{regexp.MustCompile(`\[?(?:[0-9A-Fa-f]{0,4}:){2,7}[0-9A-Fa-f]{0,4}\]?(?::\d+)?`), "[redacted-host]"},
	// user@host ssh targets
	{regexp.MustCompile(`\b[A-Za-z0-9._\-]+@[A-Za-z0-9.\-]+(:\d+)?\b`), "[redacted-host]"},
	// host:port and bare IPv4
	{regexp.MustCompile(`\b\d{1,3}(\.\d{1,3}){3}(:\d+)?\b`), "[redacted-host]"},
}

// redactLine masks sensitive content for any artifact that leaves the machine.
// Defaults to masking on ambiguity (never passes suspicious content through).
func redactLine(s string) string {
	for _, rule := range redactRules {
		s = rule.re.ReplaceAllString(s, rule.repl)
	}
	return s
}
