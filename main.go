package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// AppVersion is the conduit release version shown to users.
// Populated at build time via -ldflags "-X main.AppVersion=$(VERSION)".
// Defaults to "dev" so unreleased builds don't masquerade as a real
// release and don't trigger the update notifier.
var AppVersion = "dev"

// GitCommit and BuildTime are stamped at build time.
var GitCommit = "unknown"
var BuildTime = "unknown"

// ── config ────────────────────────────────────────────────────────────────────

var (
	captureMode     bool
	setupMode       bool
	sqlQuery        string
	sshHost         string
	sshUser         string
	sshKeyPath      string
	itemDataPath    string
	scripCurrencyID int
	dbPort          int
	dbUser          string
	dbPass          string
	dbName          string
	dbSchema        string
	listenAddr      string
)

func loadDotEnv() {
	f, err := os.Open(".env")
	if err != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if len(v) >= 2 && ((v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'')) {
			v = v[1 : len(v)-1]
		}
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
}

// envOr returns the environment variable value if set, otherwise def.
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envIntOr(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func init() {
	loadDotEnv()
	flag.StringVar(&sshHost, "host", envOr("SSH_HOST", "192.168.0.72:22"), "SSH host:port")
	flag.StringVar(&sshUser, "user", envOr("SSH_USER", "dune"), "SSH user")
	flag.StringVar(&sshKeyPath, "key", envOr("SSH_KEY", ""), "SSH private key path (auto-detected if empty)")
	flag.StringVar(&itemDataPath, "itemdata", envOr("ITEM_DATA", ""), "Item data JSON path")
	flag.IntVar(&scripCurrencyID, "scripcurrency", envIntOr("SCRIP_CURRENCY", 1), "Scrip currency id")
	flag.IntVar(&dbPort, "dbport", envIntOr("DB_PORT", 15432), "PostgreSQL port inside the cluster")
	flag.StringVar(&dbUser, "dbuser", envOr("DB_USER", "dune"), "PostgreSQL user")
	flag.StringVar(&dbPass, "dbpass", envOr("DB_PASS", ""), "PostgreSQL password")
	flag.StringVar(&dbName, "dbname", envOr("DB_NAME", "dune"), "PostgreSQL database name")
	flag.StringVar(&dbSchema, "schema", envOr("DB_SCHEMA", "dune"), "PostgreSQL schema")
	flag.StringVar(&listenAddr, "addr", envOr("LISTEN_ADDR", ":8080"), "HTTP listen address")
	flag.BoolVar(&captureMode, "capture", false, "Capture RabbitMQ messages (grant + notifications) and print to stdout")
	flag.BoolVar(&setupMode, "setup", false, "Interactive setup wizard — writes .env from SSH autodiscovery")
	flag.StringVar(&sqlQuery, "sql", "", "Run a SQL query and print results to stdout, then exit")
}

func resolveKeyPath() string {
	if sshKeyPath != "" {
		return sshKeyPath
	}
	candidates := []string{
		"%LOCALAPPDATA%/DuneSandboxServer/sshKey", // per-user app data folder on windows
		"./sshKey", // in folder with the binary, for easy drag-and-drop usage
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil { // #nosec G703 -- paths are hardcoded candidates, not user input
			return p
		}
	}
	return candidates[0]
}

func resolveItemDataPath() string {
	if itemDataPath != "" {
		return itemDataPath
	}
	candidates := []string{"./item-data.json"}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func resolveTagsDataPath() string {
	candidates := []string{"./tags-data.json"}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

var tagsData tagsDataFile

func loadTagsData() error {
	path := resolveTagsDataPath()
	if path == "" {
		return fmt.Errorf("tags-data.json not found (looked in ./ and ../) — contract picker will be empty")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read tags data %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &tagsData); err != nil {
		return fmt.Errorf("parse tags data %s: %w", path, err)
	}
	return nil
}

var itemData itemDataFile

func loadItemData() error {
	path := resolveItemDataPath()
	if path == "" {
		return fmt.Errorf("item-data.json not found (looked in ./ and ../) — item grant features will be broken")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read item data %s: %w", path, err)
	}
	var parsed itemDataFile
	if err := json.Unmarshal(data, &parsed); err != nil {
		return fmt.Errorf("parse item data %s: %w", path, err)
	}
	normalizedItems := make(map[string]itemRule, len(parsed.Items))
	for k, v := range parsed.Items {
		normalizedItems[strings.ToLower(k)] = v
	}
	parsed.Items = normalizedItems
	normalizedNames := make(map[string]string, len(parsed.Names))
	for k, v := range parsed.Names {
		normalizedNames[strings.ToLower(k)] = v
	}
	parsed.Names = normalizedNames
	itemData = parsed
	return nil
}

// ── main ──────────────────────────────────────────────────────────────────────

func needsSetup() bool {
	_, err := os.Stat(".env")
	if os.IsNotExist(err) {
		return true
	}
	// .env exists but is empty or missing the discovered DB password.
	return dbPass == ""
}

func main() {
	flag.Parse()

	// Explicit -setup flag: reconfigure and exit (don't start server).
	if setupMode {
		runSetup()
		return
	}

	if captureMode {
		if msg, ok := cmdConnect().(msgConnect); ok && msg.err != nil {
			fmt.Fprintln(os.Stderr, "SSH connect:", msg.err)
			os.Exit(1)
		}
		runCapture()
		return
	}

	if sqlQuery != "" {
		if msg, ok := cmdConnect().(msgConnect); ok && msg.err != nil {
			fmt.Fprintln(os.Stderr, "connect:", msg.err)
			os.Exit(1)
		}
		if msg, ok := cmdRunSQL(sqlQuery)().(msgSQL); ok {
			if msg.err != nil {
				fmt.Fprintln(os.Stderr, msg.err)
				os.Exit(1)
			}
			fmt.Println(msg.result)
		}
		return
	}

	if err := loadItemData(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := loadTagsData(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Auto-run setup wizard when no .env exists — setup leaves us connected.
	alreadyConnected := false
	if needsSetup() {
		runSetup()
		alreadyConnected = true
		fmt.Println()
		fmt.Printf("Starting server on %s...\n", listenAddr)
	}

	defer func() {
		if globalDB != nil {
			globalDB.Close()
		}
		if globalSSH != nil {
			globalSSH.Close()
		}
	}()

	if !alreadyConnected {
		// Connect synchronously (SSH + DB).
		if msg, ok := cmdConnect().(msgConnect); ok && msg.err != nil {
			fmt.Fprintln(os.Stderr, "connect:", msg.err)
			fmt.Fprintln(os.Stderr, "Starting server anyway — use /api/v1/reconnect to retry")
		} else {
			if msg, ok := cmdFetchItemTemplates().(msgItemTemplates); ok {
				mergeItemTemplates(msg.templates)
			}
		}
	} else {
		// Already connected by setup; just populate item templates.
		if msg, ok := cmdFetchItemTemplates().(msgItemTemplates); ok {
			mergeItemTemplates(msg.templates)
		}
	}

	startServer(listenAddr)
}
