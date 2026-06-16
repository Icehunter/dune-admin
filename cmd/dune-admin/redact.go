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
	{regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._\-]+`), "Bearer [redacted-token]"},
	{regexp.MustCompile(`(?i)(serviceauthtoken|token|api[_-]?key|key|password|passwd|secret)\s*[=:]\s*\S+`), "$1=[redacted-token]"},
	{regexp.MustCompile(`(?i)(account_id|player_id|fls_id|owner_id)\s*[=:]\s*\d+`), "$1=[redacted-id]"},
	{regexp.MustCompile(`(?i)(/home/|/Users/|C:\\Users\\)[^\s"']+`), "[redacted-path]"},
	// IPv6 (compressed or full, optional brackets, optional :port). Runs before the
	// IPv4/user@host rules so bracketed [ipv6]:port and bare forms are caught first.
	{regexp.MustCompile(`\[?(?:[0-9A-Fa-f]{0,4}:){2,7}[0-9A-Fa-f]{0,4}\]?(?::\d+)?`), "[redacted-host]"},
	{regexp.MustCompile(`\b[A-Za-z0-9._\-]+@[A-Za-z0-9.\-]+(:\d+)?\b`), "[redacted-host]"},
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
