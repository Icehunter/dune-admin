package main

// Msg and Cmd replace charm.land/bubbletea/v2's tea.Msg and tea.Cmd so that
// db.go and ssh.go can drop the bubbletea dependency while keeping their
// existing return-type signatures.
type Msg = any
type Cmd = func() Msg
