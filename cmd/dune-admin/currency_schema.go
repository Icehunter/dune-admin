package main

// Currency handling across the Dune Awakening 1.5.3 schema change.
//
// Game build 1.5.3 (CL 1867715) retyped
// dune.player_virtual_currency_balances.currency_id from smallint to
//
//	CREATE TYPE VirtualWalletType AS ENUM ('Solaris', 'HouseCredit');
//
// and dropped dune.get_solaris_id(), which dune-admin had used to identify the
// Solaris row. dune.adjust_player_virtual_currency_balance's second parameter
// changed type with it. See #333 / #334.
//
// Read queries select currency_id::text rather than branching on the server's
// build: the cast renders '0'/'1' on a pre-1.5.3 smallint column and
// 'Solaris'/'HouseCredit' on the 1.5.3 enum, so one scan target and one
// classifier cover both. Writes have to name the enum type outright, because
// no integer literal resolves against the new function signature.

const (
	// virtualWalletSolaris and virtualWalletHouseCredit are the labels of the
	// 1.5.3 VirtualWalletType enum, in declaration order. HouseCredit is the
	// currency dune-admin has always surfaced as "scrip".
	virtualWalletSolaris     = "Solaris"
	virtualWalletHouseCredit = "HouseCredit"

	// legacySolarisCurrencyID and legacyScripCurrencyID are the smallint ids the
	// same two currencies used before 1.5.3. They survive as the shape of the
	// -scripcurrency flag and the ScripCurrency config key.
	legacySolarisCurrencyID = 0
	legacyScripCurrencyID   = 1

	// solarisCurrencyArg and scripCurrencyArg are SQL literals for the currency
	// argument of dune.adjust_player_virtual_currency_balance.
	solarisCurrencyArg = `'` + virtualWalletSolaris + `'::dune.virtualwallettype`

	// solarisTextPredicate matches the Solaris row on either schema. Used in
	// aggregate CASE expressions where no bind parameter is available.
	solarisTextPredicate = `currency_id::text IN ('` + virtualWalletSolaris + `', '0')`
)

// solarisTextPredicateFor renders solarisTextPredicate against a table alias,
// for the joined queries where currency_id needs qualifying.
func solarisTextPredicateFor(alias string) string {
	return alias + `.` + solarisTextPredicate
}

// virtualWalletLabel maps a legacy numeric currency id onto its 1.5.3 enum
// label, so an existing config file carrying `ScripCurrency: 1` keeps selecting
// the same currency. Only 0 was ever Solaris; every other id was the second
// currency, which 1.5.3 names HouseCredit.
func virtualWalletLabel(id int) string {
	if id == legacySolarisCurrencyID {
		return virtualWalletSolaris
	}
	return virtualWalletHouseCredit
}

// currencyIsSolaris classifies a currency_id that was selected as text.
// It accepts both the 1.5.3 enum label and the pre-1.5.3 numeric id so the
// read path works against either build. Anything else — including an empty
// string from a NULL — is treated as not-Solaris and therefore falls into the
// scrip bucket, which is the safe direction: a miscounted scrip balance is a
// display error, whereas miscounting it as Solaris would corrupt the economy
// totals on the dashboard.
func currencyIsSolaris(text string) bool {
	switch text {
	case virtualWalletSolaris, "0":
		return true
	default:
		return false
	}
}
