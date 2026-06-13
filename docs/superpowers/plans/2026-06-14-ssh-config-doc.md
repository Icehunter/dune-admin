# SSH Config Documentation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Write a standalone, cross-platform SSH-config guide (`SETUP_SSH.md`) covering modular `config.d` structure, ProxyJump chains, per-platform ssh-agent/passphrase handling, and ControlMaster multiplexing, plus a dune-admin `command`-mode section — and link it from the README.

**Architecture:** A single Markdown document at repo root, layered as Part 1 (general, shareable with non-dune-admin colleagues) and Part 2 (dune-admin `command`-mode). All examples use anonymized placeholder hosts/IPs. The README gains one pointer line; its config-reference table stays canonical.

**Tech Stack:** Markdown, linted with `markdownlint-cli2` via `make lint-md`. No code, no Go tests — verification is lint + a privacy grep + a single-H1 check.

---

## Conventions for this plan

**Anonymization (MANDATORY — see spec "Privacy Constraint"):**

- Aliases: `gateway`, `jumphost`, `target`, `mesh-host`, `vm-target`.
- Domains: `example.com`, `*.example.internal`.
- IPs: RFC 5737 docs ranges `192.0.2.0/24`, `198.51.100.0/24`, `203.0.113.0/24`; RFC 1918 `10.0.0.0/8`.
- Users: `alice`, `admin`, `deploy`.
- **Never** use any real token from the author's setup. Forbidden substrings (checked in Task 5):
  `muehmer`, `nebula`, `hadesnet`, `vm-dune`, `vm-jh-01`, `eros`, `perses`, `zeus`, `sakura`,
  `mars`, `nyx`, `ares`, `hera`, `hermes`, `192.168.`.

**Markdown style (see `.claude/rules/documentation.md`):** ATX headers, exactly one H1,
blank line before/after headers/lists/code fences, `-` for bullets, every code fence has a
language. Use `text` as the fence language for `ssh_config` snippets (no dedicated lexer
needed), `bash` for shell, `powershell` for Windows.

---

## File Structure

- **Create:** `SETUP_SSH.md` (repo root) — the entire guide. One file: the document is a
  linear read and splitting it would fragment the narrative.
- **Modify:** `README.md` — add one pointer line in the SSH section (near line 118).

---

## Task 1: Document skeleton + introduction

**Files:**

- Create: `SETUP_SSH.md`

- [ ] **Step 1: Write the skeleton with the H1, intro, and all section headers**

Create `SETUP_SSH.md` with exactly this scaffold (content for each `##`/`###` is filled in
by later tasks; leave the headers in place now so the structure is locked):

```markdown
# SSH Config Guide

A practical, cross-platform guide to building a maintainable `~/.ssh/config`: a modular
`config.d` layout, jumphost (ProxyJump) chains, loading passphrase-protected keys into your
platform's ssh-agent, and connection multiplexing. Part 1 is general and stands alone. Part 2
covers how dune-admin's `command`-mode SSH transport reuses this same config.

For the dune-admin config reference (env vars, flags, key lookup order), see the table in
[README.md](README.md).

## Part 1 — General SSH config (cross-platform)

### Where the config lives

### Building blocks

### Modular structure with Include

### Jumphosts and ProxyJump

### ssh-agent and passphrase-protected keys

### Connection multiplexing (ControlMaster)

## Part 2 — dune-admin command-mode

### command-mode uses your SSH config

### Troubleshooting
```

- [ ] **Step 2: Verify it lints and has exactly one H1**

Run:

```bash
make lint-md
grep -c '^# ' SETUP_SSH.md
```

Expected: `lint-md` passes (exit 0); the `grep -c` prints `1`.

- [ ] **Step 3: Commit**

```bash
git add SETUP_SSH.md
git commit -m "docs(ssh): add SETUP_SSH guide skeleton"
```

---

## Task 2: Part 1 — config location, building blocks, Include structure

**Files:**

- Modify: `SETUP_SSH.md`

- [ ] **Step 1: Fill the "Where the config lives" section**

Replace the empty `### Where the config lives` header's body with content covering:

- The file is `~/.ssh/config`; the directory needs mode `700` and config/key files mode
  `600` — ssh refuses to use a private key with loose permissions.
- Per-platform locations:
  - Linux / macOS: `~/.ssh/`
  - Windows (native): `%USERPROFILE%\.ssh\` (i.e. `C:\Users\<you>\.ssh\`)
  - WSL2: a **separate** `~/.ssh/` inside the Linux home — it is *not* the Windows profile's
    `.ssh`. State explicitly that native-Windows and WSL2 keep two independent configs.

Include this permissions snippet:

```bash
chmod 700 ~/.ssh
chmod 600 ~/.ssh/config ~/.ssh/id_ed25519
```

- [ ] **Step 2: Fill the "Building blocks" section**

Cover `Host`, `HostName`, `User`, `Port`, `IdentityFile`, and a global `Host *` block for
defaults. Explain **first-match-wins**: for each parameter ssh takes the *first* matching
value in file order, so specific hosts must come before broad wildcards. Call this out as the
most common pitfall. Include:

```text
Host gateway
    HostName gateway.example.com
    User admin
    Port 22
    IdentityFile ~/.ssh/id_ed25519

Host *
    User alice
```

Add a one-line note: because of first-match-wins, the specific `Host gateway` block above
must appear before `Host *`, or `User admin` would never win over the `Host *` default.

- [ ] **Step 3: Fill the "Modular structure with Include" section**

Describe a lean main file that only includes fragments, plus a global block. Show:

```text
# ~/.ssh/config
Include config.d/managed/*.conf
Include config.d/*.conf

Host *
    User alice
    SetEnv TERM=xterm-256color
```

Then explain:

- One fragment per network/purpose under `config.d/` (e.g. `config.d/work-net.conf`,
  `config.d/lab.conf`).
- A `config.d/managed/` subdir included *first* for tool-generated fragments you don't edit
  by hand.
- A parallel `config.d.disabled/` directory (not included) to toggle a whole fragment set off
  without deleting it — just move files in or out.
- Include paths are relative to `~/.ssh`. Combined with first-match-wins, earlier includes
  win for a given parameter, so order your `Include` lines deliberately.

Show one example fragment:

```text
# config.d/lab.conf
Host mesh-host
    HostName mesh-host.example.internal
    User alice
```

- [ ] **Step 4: Verify lint + privacy**

Run:

```bash
make lint-md
```

Expected: passes (exit 0).

- [ ] **Step 5: Commit**

```bash
git add SETUP_SSH.md
git commit -m "docs(ssh): config location, building blocks, Include structure"
```

---

## Task 3: Part 1 — ProxyJump, ssh-agent, ControlMaster

**Files:**

- Modify: `SETUP_SSH.md`

- [ ] **Step 1: Fill the "Jumphosts and ProxyJump" section**

Explain reaching a host that isn't directly routable by hopping through one or more
intermediates. Show a two-hop chain (`target` → `jumphost` → `gateway`):

```text
# config.d/internal.conf
Host gateway
    HostName gateway.example.com
    User admin

Host jumphost
    HostName 192.0.2.10
    User admin
    ProxyJump gateway

Host target
    HostName 198.51.100.20
    User deploy
    ProxyJump jumphost
```

Then `ssh target` transparently connects through `gateway` then `jumphost`. Cover:

- Inline form without config: `ssh -J gateway,jumphost deploy@198.51.100.20`.
- `ProxyJump` is the modern, declarative option; reach for a custom `ProxyCommand` only when
  you need a non-ssh tunnel (e.g. a corporate proxy binary) that `ProxyJump` can't express.

- [ ] **Step 2: Fill the "ssh-agent and passphrase-protected keys" section**

Intro sentence: a passphrase-protected key is decrypted once and held by an agent so you
aren't re-prompted every connection. One `####` sub-section per OS:

````markdown
#### Linux

Most desktops start an agent via systemd user services or the keyring. If `ssh-add -l` says
"Could not open a connection", start one and add your key:

```bash
eval "$(ssh-agent -s)"
ssh-add ~/.ssh/id_ed25519
```

Set `AddKeysToAgent yes` under `Host *` to load keys into the running agent automatically on
first use.

#### Windows (native)

Enable the built-in **OpenSSH Authentication Agent** service once, from an elevated
PowerShell, then add your key:

```powershell
Set-Service ssh-agent -StartupType Automatic
Start-Service ssh-agent
ssh-add $env:USERPROFILE\.ssh\id_ed25519
```

The service holds the decrypted key in the Windows credential store, so the passphrase is
entered once and survives reboots.

#### WSL2

WSL2 runs its own agent inside the Linux distro — set it up exactly as in the Linux section
above. Bridging to the native-Windows agent (so both share one key) is possible with a helper
like `npiperelay`, but that is an advanced, optional setup and out of scope here.

#### macOS

macOS integrates ssh with the login Keychain. Add to `Host *`:

```text
Host *
    AddKeysToAgent yes
    UseKeychain yes
```

Then store the passphrase in the Keychain once:

```bash
ssh-add --apple-use-keychain ~/.ssh/id_ed25519
```

The Keychain re-supplies the passphrase automatically on later logins.
````

- [ ] **Step 3: Fill the "Connection multiplexing (ControlMaster)" section**

Explain that multiplexing reuses one authenticated TCP connection for many sessions, making
the 2nd..Nth connection near-instant. Show:

```text
Host *
    ControlMaster auto
    ControlPath ~/.ssh/cm-%C
    ControlPersist 60s
```

Cover:

- `%C` hashes the connection parameters into the socket name, avoiding the ~104-char
  `ControlPath` length limit.
- `ControlPersist 60s` keeps the master alive briefly after the last session closes.
- **Windows (native) does not support ControlMaster** (it has no Unix-domain sockets); WSL2,
  Linux, and macOS do. On native Windows, omit these options — each connection stands alone.

- [ ] **Step 4: Verify lint**

Run:

```bash
make lint-md
```

Expected: passes (exit 0).

- [ ] **Step 5: Commit**

```bash
git add SETUP_SSH.md
git commit -m "docs(ssh): ProxyJump chains, per-platform agent, ControlMaster"
```

---

## Task 4: Part 2 — dune-admin command-mode + Troubleshooting

**Files:**

- Modify: `SETUP_SSH.md`

- [ ] **Step 1: Fill the "command-mode uses your SSH config" section**

Explain that dune-admin's `command` SSH mode shells out to the OS `ssh` client, so it inherits
your entire `~/.ssh/config`: host aliases, `ProxyJump` chains, the agent, and `Include`d
fragments. You define the target host once in your config and point dune-admin at the alias.

Show a target fragment and the invocation:

```text
# config.d/dune.conf
Host vm-target
    HostName 198.51.100.20
    User deploy
    ProxyJump jumphost
```

```bash
dune-admin -host vm-target -ssh-mode command
```

Cover:

- `-host` (env `SSH_HOST`) may be a plain `host:port` or a `~/.ssh/config` alias like
  `vm-target`; the alias resolves `HostName`, `User`, and the `ProxyJump` chain for you.
- The full env/flag reference (`SSH_HOST`, `SSH_MODE`, `SSH_EXTRA_OPTS`, key lookup) lives in
  the [README.md](README.md) config table — link, don't duplicate.
- Windows note: `command`-mode runs without ControlMaster multiplexing (see
  [Connection multiplexing](#connection-multiplexing-controlmaster)) — functional, just one
  connection per operation.

- [ ] **Step 2: Fill the "Troubleshooting" section**

A `-` list of the common failures and the fix/diagnostic for each:

- **See what ssh actually does:** `ssh -v vm-target` (add more `-v` for detail) prints config
  resolution and each ProxyJump hop.
- **"Bad permissions" / key ignored:** `chmod 700 ~/.ssh` and `chmod 600 ~/.ssh/config
  ~/.ssh/id_ed25519`.
- **Wrong `User`/`HostName` applied:** first-match-wins — a broad `Host *` (or an earlier
  `Include`) set the value before your specific block. Reorder so the specific block wins.
- **Repeated passphrase prompts:** the agent isn't running or the key isn't loaded — check
  with `ssh-add -l`, then load it (see the agent section for your OS).
- **Auth fails at an intermediate hop:** ProxyJump authenticates at *each* hop; ensure your
  key/agent reaches `gateway` and `jumphost`, not just `target`.

- [ ] **Step 3: Verify lint**

Run:

```bash
make lint-md
```

Expected: passes (exit 0).

- [ ] **Step 4: Commit**

```bash
git add SETUP_SSH.md
git commit -m "docs(ssh): dune-admin command-mode section + troubleshooting"
```

---

## Task 5: README link + final verification

**Files:**

- Modify: `README.md`

- [ ] **Step 1: Add a pointer line in the README SSH section**

Open `README.md` and locate the SSH config table (around lines 115–119, the rows for
`SSH_HOST` / `SSH_MODE` / etc.). Immediately after that table, add one line:

```markdown
> For building your `~/.ssh/config` (modular `config.d`, jumphost chains, per-platform
> ssh-agent setup, multiplexing), see [SETUP_SSH.md](SETUP_SSH.md).
```

Match the exact phrasing/punctuation style of the surrounding README (e.g. how the other
`SETUP_*.md` links are introduced near lines 46–49); adjust only if the blockquote clashes
with the local style — a plain sentence is fine too.

- [ ] **Step 2: Privacy grep — no real tokens leaked**

Run (must produce NO output):

```bash
grep -rEi 'muehmer|nebula|hadesnet|vm-dune|vm-jh-01|\beros\b|perses|zeus|sakura|\bmars\b|\bnyx\b|\bares\b|\bhera\b|hermes|192\.168\.' SETUP_SSH.md
```

Expected: empty output (exit 1 from grep = no matches). If anything prints, replace the real
token with an anonymized placeholder and re-run.

- [ ] **Step 3: Final lint of both files**

Run:

```bash
make lint-md
```

Expected: passes (exit 0) for both `SETUP_SSH.md` and `README.md`.

- [ ] **Step 4: Verify the README link resolves**

Run:

```bash
test -f SETUP_SSH.md && grep -q 'SETUP_SSH.md' README.md && echo OK
```

Expected: prints `OK`.

- [ ] **Step 5: Commit**

```bash
git add README.md
git commit -m "docs: link SETUP_SSH from README SSH section"
```

---

## Self-Review (completed during planning)

**Spec coverage:**

- Placement at root as `SETUP_SSH.md`, README pointer → Task 1, Task 5.
- Privacy constraint (anonymized examples, forbidden tokens) → plan Conventions + Task 5 grep.
- Part 1 §1 config location/permissions/per-platform/WSL2 → Task 2 Step 1.
- Part 1 §2 building blocks + first-match-wins → Task 2 Step 2.
- Part 1 §3 Include / managed / disabled → Task 2 Step 3.
- Part 1 §4 ProxyJump chain + ProxyCommand distinction → Task 3 Step 1.
- Part 1 §5 ssh-agent per OS (Linux, Windows service, WSL2, macOS Keychain) → Task 3 Step 2.
- Part 1 §6 ControlMaster + Windows-native limitation → Task 3 Step 3.
- Part 2 §7 command-mode inherits config + README cross-ref → Task 4 Step 1.
- Part 2 §8 troubleshooting → Task 4 Step 2.
- Success criterion "README gains exactly one pointer line, no duplication" → Task 5 Step 1.

No gaps found.

**Placeholder scan:** No TBD/TODO/"handle edge cases". All config and command blocks are
concrete. Prose sections are specified as explicit content requirements, not vague directives.

**Consistency:** Section headers in Task 1's skeleton match the headers filled in Tasks 2–4
verbatim (`Where the config lives`, `Building blocks`, `Modular structure with Include`,
`Jumphosts and ProxyJump`, `ssh-agent and passphrase-protected keys`,
`Connection multiplexing (ControlMaster)`, `command-mode uses your SSH config`,
`Troubleshooting`). The anchor used in Task 4's Windows note
(`#connection-multiplexing-controlmaster`) is the GitHub-slug of the Task 3 Step 3 header.
Placeholder aliases (`gateway`, `jumphost`, `target`, `vm-target`, `mesh-host`) are used
consistently across tasks.
