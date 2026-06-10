package main

// presenceTracker detects player join events by diffing the set of online
// accounts across successive observations. The first observation seeds a silent
// baseline so a dune-admin (re)start does not re-fire on-join actions (e.g. the
// MOTD) for everyone already in-game; a player who goes offline and returns is a
// new join. Keyed on account id (always present), which is also what the whisper
// path consumes.
//
// Not safe for concurrent use: the scanner calls observe() serially from its
// single goroutine.
type presenceTracker struct {
	seen   map[int64]struct{}
	seeded bool
}

func newPresenceTracker() *presenceTracker {
	return &presenceTracker{seen: map[int64]struct{}{}}
}

// observe records the currently-online accounts and returns those newly online
// since the previous observation (join events). The first call returns no joins
// (it only seeds the baseline).
func (p *presenceTracker) observe(online []welcomeAccount) []welcomeAccount {
	current := make(map[int64]struct{}, len(online))
	var joins []welcomeAccount
	for _, acc := range online {
		current[acc.AccountID] = struct{}{}
		if !p.seeded {
			continue
		}
		if _, ok := p.seen[acc.AccountID]; !ok {
			joins = append(joins, acc)
		}
	}
	p.seen = current
	p.seeded = true
	return joins
}

// reset re-arms the baseline so the next observe is silent. Used when the MOTD
// feature is toggled off (and later on) so currently-online players are not
// messaged on the flip — only genuine future joins are.
func (p *presenceTracker) reset() {
	p.seen = map[int64]struct{}{}
	p.seeded = false
}
