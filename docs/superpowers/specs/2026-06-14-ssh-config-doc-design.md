# SSH Config Documentation — Design Spec

**Date:** 2026-06-14
**Status:** Approved (design)
**Artifact:** A new standalone guide at `SETUP_SSH.md` (repo root), linked from `README.md`.

## Goal

Provide a layered, cross-platform guide to building a maintainable `~/.ssh/config`.
Two audiences in one document:

1. **General SSH-config readers** (Part 1) — teammates on Windows and macOS who lack a
   solid mental model of `ssh_config`. This part stands on its own and is shareable
   without any dune-admin context.
2. **dune-admin operators** (Part 2) — how the `command`-mode SSH executor consumes the
   user's `~/.ssh/config` (ProxyJump chains, agent, includes).

Non-goal: replacing the existing README config-reference table (env vars / flags / key
lookup order, README lines ~115–257). The guide links to it; it does not duplicate it.

## Placement & Linking

- New file: `SETUP_SSH.md` in the repo root.
- Follows the established repo convention: setup/operator guides live at root as
  `SETUP_<TOPIC>.md` (`SETUP_AMP.md`, `SETUP_DOCKER.md`, `SETUP_KUBECTL.md`,
  `SETUP_LOCAL.md`) and are linked from the README. `documentation.md` cites
  `SETUP_KUBECTL.md` as the naming model.
- `README.md` SSH section (around line 118) gains a one-line pointer to the guide. No
  content moves out of the README — the table stays as the canonical reference.
- Not an ADR: this is operator/contributor guidance, not an architecture decision.

## Privacy Constraint

All examples use generic placeholder names and reserved/documentation values — never the
user's real hosts, IPs, or domains. Conventions for examples:

- Aliases: `target`, `jumphost`, `gateway`, `mesh-host`.
- Domains: `example.com` / `*.example.internal`.
- IPs: RFC 5737 documentation ranges (`192.0.2.0/24`, `198.51.100.0/24`, `203.0.113.0/24`)
  and RFC 1918 private ranges (`10.0.0.0/8`) where a private-network feel is needed.
- Users: `alice` / `admin` / `deploy`.

## Structure (derived from the user's real layout, anonymized)

The user's actual pattern — verified, reproduced generically:

- A lean `~/.ssh/config`: only `Include config.d/managed/*.conf`, `Include config.d/*.conf`,
  and a global `Host *` block (sets `User`, `SetEnv`).
- Thematic fragments per network/purpose in `config.d/` (one file for mesh hosts, one for a
  target VM group behind a jumphost chain, one for a local-network variant).
- A `managed/` subdir (included first, for tool-generated fragments) and a parallel
  `*.disabled/` dir to toggle fragment sets off without deleting them.
- Multi-hop ProxyJump chains: `target → jumphost → gateway` (two hops in the real setup).

## Document Outline

### Introduction

Purpose, audience, what is covered. Pointer to the README reference table.

### Part 1 — General SSH config (cross-platform)

1. **Where the config lives**
   - File: `~/.ssh/config`; directory and key permissions (`700` dir, `600` files) and why
     strict modes matter (ssh refuses loose-permissioned keys).
   - Per-platform paths: Linux/macOS `~/.ssh/`; Windows native `%USERPROFILE%\.ssh\`
     (`C:\Users\<user>\.ssh\`); **WSL2 has its own Linux `~/.ssh/`, separate from the
     Windows profile** — call out that these are two distinct configs.

2. **Building blocks**
   - `Host`, `HostName`, `User`, `Port`, `IdentityFile`.
   - Global defaults via a `Host *` block.
   - **First-match-wins** semantics: for each parameter the *first* matching value in file
     order is used; put specific hosts before broad wildcards. Flag this as the single most
     common pitfall.

3. **Modular structure with `Include`**
   - Lean main file + `Include config.d/*.conf`; thematic fragments.
   - `managed/` (included first, tool-generated) and `*.disabled/` (toggle without delete).
   - Include path resolution (relative to `~/.ssh`) and interaction with first-match-wins
     (earlier includes win for a given parameter).

4. **Jumphosts / ProxyJump**
   - Multi-hop chain `target → jumphost → gateway` using `ProxyJump` (and inline `-J`).
   - When `ProxyJump` suffices vs. when a custom `ProxyCommand` is needed.
   - Worked anonymized example mirroring the user's two-hop chain.

5. **ssh-agent & passphrase-protected keys** (one sub-point per OS)
   - **Linux** — agent via systemd user service / desktop keyring; `ssh-add`;
     `AddKeysToAgent yes` to load on first use.
   - **Windows (native)** — enable the *OpenSSH Authentication Agent* service:
     `Set-Service ssh-agent -StartupType Automatic; Start-Service ssh-agent`, then
     `ssh-add <key>`. Note the key is held by the service (Windows credential store), so the
     passphrase is entered once. This is its own callout per the user's request.
   - **WSL2** — its own agent inside the Linux distro; brief mention of two advanced options
     to reuse the Windows side for a single source of truth: calling Windows `ssh.exe` via a
     `PATH` wrapper (not a shell alias — programs like dune-admin's `command`-mode bypass
     aliases), and bridging the agent socket (e.g. npiperelay). Caveats: alias-vs-`PATH`,
     Windows path semantics, no ControlMaster. Named, not walked through.
   - **macOS** — Keychain integration: `UseKeychain yes` + `AddKeysToAgent yes` in config,
     `ssh-add --apple-use-keychain <key>`; the keychain re-supplies the passphrase
     automatically on later logins.

6. **Connection multiplexing (ControlMaster)**
   - `ControlMaster auto`, `ControlPath` (with `%C` to dodge the socket-path length limit),
     `ControlPersist`. Benefit: subsequent sessions reuse one authenticated connection.
   - **Windows native does not support it** (no Unix-domain sockets); WSL2/Linux/macOS do.

### Part 2 — dune-admin command-mode

1. **command-mode uses your `~/.ssh/config`**
   - The `command` SSH mode shells out to the OS `ssh` client and therefore inherits the
     entire config: ProxyJump chains, agent, includes, host aliases.
   - Minimal host definition needed so `SSH_HOST` points at a dune VM behind a jumphost.
   - Cross-reference `SSH_HOST` / `SSH_MODE` / `SSH_EXTRA_OPTS` in the README table.
   - Windows note: command-mode runs without multiplexing (ControlMaster skipped) — link to §6.

2. **Troubleshooting**
   - `ssh -v` / `-vv` to see config resolution and the ProxyJump chain.
   - Common failures: bad permissions, first-match-wins surprises, agent not running,
     ProxyJump authentication at an intermediate hop.

## Success Criteria

- A Windows or macOS colleague can build a working modular `~/.ssh/config` with a jumphost
  chain and a passphrase key loaded into their platform's agent, using only Part 1.
- A dune-admin operator can point `command`-mode at a VM behind a two-hop jumphost chain
  using Part 2 plus the README table.
- No real hosts, IPs, or domains appear anywhere in the guide.
- The README gains exactly one pointer line; no reference content is duplicated.

## Out of Scope

- Rewriting or relocating the README config-reference table.
- Key generation tutorials (`ssh-keygen`) beyond a one-line pointer.
- Deep WSL2↔Windows agent bridging setup (mentioned, not walked through).
- Server-side `sshd_config`, CA/certificate auth, and `Match` blocks beyond a brief mention.
