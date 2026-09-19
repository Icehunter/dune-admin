package main

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Dune Awakening 1.5.3 (game CL 1867715) changed the server database out from
// under dune-admin. Verified against the DDL the game build ships in
// DuneSandbox/Database (01_Dune.sql plus the DA-11315 upgrade script):
//
//  1. dune.get_solaris_id() was dropped outright ("-- CL 1867715 removed this").
//  2. dune.player_virtual_currency_balances.currency_id was retyped from
//     smallint to the enum VirtualWalletType AS ENUM ('Solaris','HouseCredit').
//  3. dune.adjust_player_virtual_currency_balance's second parameter changed
//     from smallint to VirtualWalletType.
//  4. dune.encrypted_accounts.platform_id (TEXT) was replaced by
//     encrypted_platform_id (Bytea).
//
// Reported as #333 (ERR ensure GM identity) and #334 (Players tab dead:
// "failed to load stats"). These tests pin each of those four contracts.

// goSourceSQL returns the contents of every non-test .go file under cmd/ and
// internal/, so a repo-wide assertion can catch a stale SQL string anywhere
// rather than only in the files a reviewer thought to check.
func goSourceSQL(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, root := range []string{".", filepath.Join("..", "..", "internal")} {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			b, readErr := os.ReadFile(path) // #nosec G304 -- walking this repo's own source tree
			if readErr != nil {
				return readErr
			}
			out[path] = string(b)
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
	if len(out) == 0 {
		t.Fatal("found no Go source files to scan; the walk roots are wrong")
	}
	return out
}

// TestNoGetSolarisIDReferences is the regression guard for #334. The function
// no longer exists on 1.5.3 servers, so any surviving call raises
// "function dune.get_solaris_id() does not exist (SQLSTATE 42883)" and takes
// out the whole dashboard stats fetch.
// Comments are allowed to name the dropped function — the migration notes in
// db.go and currency_schema.go explain why it is gone. Only live SQL counts.
func TestNoGetSolarisIDReferences(t *testing.T) {
	t.Parallel()
	for path, src := range goSourceSQL(t) {
		if !strings.Contains(src, "get_solaris_id") {
			continue
		}
		for i, line := range strings.Split(src, "\n") {
			if !strings.Contains(line, "get_solaris_id") {
				continue
			}
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			t.Errorf("%s:%d still references the dropped function dune.get_solaris_id(): %s",
				path, i+1, strings.TrimSpace(line))
		}
	}
}

// TestNoSmallintCurrencyCasts guards the second half of #334. currency_id is an
// enum on 1.5.3, so comparing it to an integer raises
// "operator does not exist: virtualwallettype = integer (SQLSTATE 42883)".
func TestNoSmallintCurrencyCasts(t *testing.T) {
	t.Parallel()
	bad := []string{
		"currency_id = $1::smallint",
		"currency_id = $2::smallint",
		"currency_id = 0",
		"currency_id = 1",
	}
	for path, src := range goSourceSQL(t) {
		for _, frag := range bad {
			if strings.Contains(src, frag) {
				t.Errorf("%s compares the VirtualWalletType enum to an integer (%q); "+
					"1.5.3 requires the enum label", path, frag)
			}
		}
	}
}

// TestVirtualWalletLabel pins the mapping from the legacy numeric currency id —
// still the shape of the -scripcurrency flag and the ScripCurrency config key —
// onto the 1.5.3 enum labels, so existing config files keep working.
func TestVirtualWalletLabel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		id   int
		want string
	}{
		{name: "0 is Solaris", id: 0, want: "Solaris"},
		{name: "1 is HouseCredit (was scrip)", id: 1, want: "HouseCredit"},
		{name: "unknown id falls back to HouseCredit", id: 7, want: "HouseCredit"},
		{name: "negative id falls back to HouseCredit", id: -1, want: "HouseCredit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := virtualWalletLabel(tt.id); got != tt.want {
				t.Errorf("virtualWalletLabel(%d) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}

// TestCurrencyIsSolaris covers the read path. Queries select currency_id::text
// so a single scan target serves both the pre-1.5.3 smallint column and the
// 1.5.3 enum; this classifies the resulting text.
func TestCurrencyIsSolaris(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		text string
		want bool
	}{
		{name: "1.5.3 enum label", text: "Solaris", want: true},
		{name: "legacy numeric solaris", text: "0", want: true},
		{name: "1.5.3 house credit", text: "HouseCredit", want: false},
		{name: "legacy numeric scrip", text: "1", want: false},
		{name: "empty string is not solaris", text: "", want: false},
		{name: "unknown label is not solaris", text: "Spice", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := currencyIsSolaris(tt.text); got != tt.want {
				t.Errorf("currencyIsSolaris(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

// TestSeedGMIdentityWritesEncryptedPlatformID is the regression guard for #333.
// dune-admin wrote encrypted_accounts.platform_id, which 1.5.3 replaced with
// encrypted_platform_id (Bytea), so startup logged
// `column "platform_id" of relation "encrypted_accounts" does not exist`.
//
// The platform id is migrated, not dropped: the value still gets written, now
// through dune.encrypt_user_data() into the Bytea column, exactly as the game's
// own DDL does it (see user_data_encryption_setup.sql, which encrypts
// encrypted_platform_id alongside encrypted_funcom_id). The dune.accounts view
// decrypts it back out as platform_id.
func TestSeedGMIdentityWritesEncryptedPlatformID(t *testing.T) {
	t.Parallel()
	db := &captureExecer{}
	if err := seedGMIdentity(context.Background(), db, gmSeedSpec()); err != nil {
		t.Fatalf("seedGMIdentity returned unexpected error: %v", err)
	}

	var accountInsert string
	for _, c := range db.calls {
		if strings.Contains(c.sql, "INSERT INTO dune.encrypted_accounts") {
			accountInsert = c.sql
			break
		}
	}
	if accountInsert == "" {
		t.Fatal("no INSERT INTO dune.encrypted_accounts found in Exec calls")
	}

	columns := accountInsert[strings.Index(accountInsert, "(")+1 : strings.Index(accountInsert, ")")]
	var names []string
	for _, col := range strings.Split(columns, ",") {
		names = append(names, strings.TrimSpace(col))
	}
	has := func(want string) bool {
		for _, n := range names {
			if n == want {
				return true
			}
		}
		return false
	}

	// The bare column name is what 1.5.3 removed.
	if has("platform_id") {
		t.Errorf("GM account INSERT still writes the dropped column platform_id; SQL:\n%s", accountInsert)
	}
	// ...and the value must still be written, to its replacement column.
	if !has("encrypted_platform_id") {
		t.Errorf("GM account INSERT does not write encrypted_platform_id; the platform id "+
			"must be migrated to the new column, not dropped. SQL:\n%s", accountInsert)
	}
	// A Bytea column needs the game's own encryption helper, not a raw string.
	if !strings.Contains(accountInsert, "dune.encrypt_user_data($4)") {
		t.Errorf("encrypted_platform_id must be written through dune.encrypt_user_data; SQL:\n%s", accountInsert)
	}
	if !has("platform_name") {
		t.Errorf("GM account INSERT dropped platform_name, which still exists on 1.5.3; SQL:\n%s", accountInsert)
	}

	// The platform id itself must be bound, not inlined, so it is encrypted.
	var bound bool
	for _, c := range db.calls {
		if strings.Contains(c.sql, "INSERT INTO dune.encrypted_accounts") {
			for _, a := range c.args {
				if a == gmIdentityPlatformID {
					bound = true
				}
			}
		}
	}
	if !bound {
		t.Errorf("GM account INSERT does not bind the platform id %q as an argument",
			gmIdentityPlatformID)
	}
}

// TestAdjustCurrencyCallsUseEnumType checks the write path against the 1.5.3
// signature adjust_player_virtual_currency_balance(BIGINT, VirtualWalletType,
// BIGINT). A smallint in the second position no longer resolves to any overload.
// Each call is inspected on its own — db.go also casts faction_id to smallint,
// which is untouched by 1.5.3 and must not trip this guard.
func TestAdjustCurrencyCallsUseEnumType(t *testing.T) {
	t.Parallel()
	const fn = "adjust_player_virtual_currency_balance("
	found := 0
	for path, src := range goSourceSQL(t) {
		for idx := 0; ; {
			at := strings.Index(src[idx:], fn)
			if at < 0 {
				break
			}
			at += idx
			idx = at + len(fn)

			// The argument list ends at the closing backtick of the SQL literal
			// or the next 200 bytes, whichever comes first — enough to cover the
			// three arguments without running into the following statement.
			end := min(at+200, len(src))
			call := src[at:end]
			found++

			if strings.Contains(call, "::smallint") {
				t.Errorf("%s passes a smallint as the currency argument to "+
					"adjust_player_virtual_currency_balance; 1.5.3 expects VirtualWalletType:\n%s", path, call)
			}
			// The cast is either written inline or supplied by solarisCurrencyArg,
			// whose own value is asserted below.
			if !strings.Contains(strings.ToLower(call), "virtualwallettype") &&
				!strings.Contains(call, "solarisCurrencyArg") {
				t.Errorf("%s calls adjust_player_virtual_currency_balance without casting the "+
					"currency argument to dune.virtualwallettype:\n%s", path, call)
			}
		}
	}
	if found == 0 {
		t.Fatal("found no adjust_player_virtual_currency_balance call sites; the scan is broken")
	}

	// Pin the shared constant the call sites rely on, so the check above cannot
	// be satisfied by a constant that stopped naming the enum.
	if !strings.Contains(strings.ToLower(solarisCurrencyArg), "virtualwallettype") {
		t.Errorf("solarisCurrencyArg = %q, want a cast to dune.virtualwallettype", solarisCurrencyArg)
	}
	if !strings.Contains(solarisCurrencyArg, virtualWalletSolaris) {
		t.Errorf("solarisCurrencyArg = %q, want it to name the %q enum label", solarisCurrencyArg, virtualWalletSolaris)
	}
}

// TestEditGuildDescriptionPassesEditorID covers a separate signature change in
// the same DDL drop. dune.edit_guild_description was dropped and recreated with
// a third, non-defaulted parameter:
//
//	edit_guild_description(in_guild_id BIGINT, in_guild_desc TEXT, in_edited_by_player_id BIGINT)
//
// which writes guilds.guild_description_edited_by. dune-admin called the old
// two-argument form, so every guild description edit failed with
// "function dune.edit_guild_description(bigint, text) does not exist". There is
// no surviving two-argument overload in the shipped DDL.
func TestEditGuildDescriptionPassesEditorID(t *testing.T) {
	t.Parallel()
	db := &captureExecer{}
	if err := cmdEditGuildDescription(context.Background(), db, 42, "a new description"); err != nil {
		t.Fatalf("cmdEditGuildDescription returned unexpected error: %v", err)
	}
	if len(db.calls) != 1 {
		t.Fatalf("expected exactly 1 Exec call, got %d", len(db.calls))
	}
	call := db.calls[0]

	if !strings.Contains(call.sql, "$3") {
		t.Errorf("edit_guild_description called without a third argument; 1.5.x requires "+
			"in_edited_by_player_id. SQL: %s", call.sql)
	}
	if len(call.args) != 3 {
		t.Fatalf("expected 3 bound args (guild id, description, editor id), got %d: %v",
			len(call.args), call.args)
	}
	if call.args[0] != int64(42) {
		t.Errorf("arg 1 = %v, want the guild id 42", call.args[0])
	}
	if call.args[1] != "a new description" {
		t.Errorf("arg 2 = %v, want the new description", call.args[1])
	}
	// The panel edits as the seeded GM persona, not as a real player, so the
	// attribution must be the GM controller actor rather than 0 (which would
	// point at no actor at all).
	if call.args[2] != gmSeedSpec().ControllerID {
		t.Errorf("arg 3 = %v, want the GM controller id %d for attribution",
			call.args[2], gmSeedSpec().ControllerID)
	}
}

// TestReturningPlayerStatusPassesInterval guards another pre-1.5.3 bug the sweep
// turned up. dune.update_returning_player_status's second parameter is an
// Interval, and dune-admin passed the bare integer literal 0. Postgres has no
// implicit integer -> interval cast, so function resolution fails outright.
// Confirmed by preparing the statement against a live 1.5.3 server:
//
//	ERROR: function dune.update_returning_player_status(unknown, integer) does not exist (SQLSTATE 42883)
//
// which made "reset returning player award" fail every time it was used.
func TestReturningPlayerStatusPassesInterval(t *testing.T) {
	t.Parallel()
	const fn = "update_returning_player_status("
	found := 0
	for path, src := range goSourceSQL(t) {
		for idx := 0; ; {
			at := strings.Index(src[idx:], fn)
			if at < 0 {
				break
			}
			at += idx
			idx = at + len(fn)
			end := min(at+120, len(src))
			call := src[at:end]
			// Only count real call sites, not the doc comment naming the proc.
			if !strings.Contains(call, "$1") {
				continue
			}
			found++
			if !strings.Contains(strings.ToUpper(call), "INTERVAL") {
				t.Errorf("%s calls update_returning_player_status without an interval "+
					"for its second argument; Postgres cannot cast an integer to interval:\n%s", path, call)
			}
		}
	}
	if found == 0 {
		t.Fatal("found no update_returning_player_status call sites; the scan is broken")
	}
}

// TestWorldPartitionQueriesUsePartitionID guards a bug found during the 1.5.3
// sweep that predates it: dune.world_partition has no "id" column — its primary
// key is partition_id. Both teleport fallbacks selected "id", so the query
// errored, and because the error is deliberately discarded the caller silently
// continued with partition 0.
func TestWorldPartitionQueriesUsePartitionID(t *testing.T) {
	t.Parallel()
	for path, src := range goSourceSQL(t) {
		if !strings.Contains(src, "dune.world_partition") {
			continue
		}
		for i, line := range strings.Split(src, "\n") {
			if !strings.Contains(line, "dune.world_partition") {
				continue
			}
			// "SELECT id FROM" / "ORDER BY id" — the bare column, not partition_id.
			if regexp.MustCompile(`\bSELECT\s+id\b|\bORDER\s+BY\s+id\b`).MatchString(line) {
				t.Errorf("%s:%d selects a non-existent world_partition column \"id\"; "+
					"the primary key is partition_id: %s", path, i+1, strings.TrimSpace(line))
			}
		}
	}
}
