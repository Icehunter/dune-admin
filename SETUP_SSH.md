# SSH Config Guide

A practical, cross-platform guide to building a maintainable `~/.ssh/config`: a modular
`config.d` layout, jumphost (ProxyJump) chains, loading passphrase-protected keys into your
platform's ssh-agent, and connection multiplexing. Part 1 is general and stands alone. Part 2
covers how dune-admin's `command`-mode SSH transport reuses this same config.

For the dune-admin config reference (env vars, flags, key lookup order), see the table in
[README.md](README.md).

## Part 1 — General SSH config (cross-platform)

### Where the config lives

The per-user client config is a single file, `~/.ssh/config`. On POSIX systems (Linux/macOS/WSL),
OpenSSH is strict about permissions: the `.ssh` directory should be `700`, and private keys
typically `600` (ssh will warn `Permissions ... are too open` / `Bad permissions` and ignore the key).
On Windows, OpenSSH uses NTFS ACLs instead of POSIX modes — ensure only your user can read the
`.ssh` directory and private key.

```bash
chmod 700 ~/.ssh
chmod 600 ~/.ssh/config ~/.ssh/id_ed25519
```

Where the file lives per platform:

- **Linux / macOS:** `~/.ssh/`
- **Windows (native):** `%USERPROFILE%\.ssh\`, i.e. `C:\Users\<you>\.ssh\`
- **WSL2:** a **separate** `~/.ssh/` inside the Linux home — it is *not* the Windows profile's
  `.ssh`. Native Windows and WSL2 keep two completely independent configs and key sets; a key
  loaded in one is not visible to the other unless you bridge the agents (see below).

### Building blocks

A host entry maps a short alias to connection parameters:

```text
Host gateway
    HostName gateway.example.com
    User admin
    Port 22
    IdentityFile ~/.ssh/id_ed25519

Host *
    User alice
```

- `Host` — the alias you type (`ssh gateway`). Patterns like `*` match many hosts.
- `HostName` — the real DNS name or IP to connect to.
- `User`, `Port`, `IdentityFile` — the login user, port, and key for that host.
- `Host *` — a catch-all block for defaults applied to every connection.

The single most important rule is **first-match-wins**: for each parameter, ssh uses the
*first* matching value in file order. Specific host blocks must therefore appear **before**
broad wildcards. In the example above, `Host gateway` must come before `Host *`, otherwise the
`Host *` default of `User alice` would win and `User admin` would never apply.

### Modular structure with Include

A large flat config is hard to maintain. Keep the main file lean — just includes plus global
defaults — and split everything else into purpose-specific fragments:

```text
# ~/.ssh/config
Include config.d/managed/*.conf
Include config.d/*.conf

Host *
    User alice
    SetEnv TERM=xterm-256color
```

- One fragment per network or purpose under `config.d/`, e.g. `config.d/work-net.conf` or
  `config.d/lab.conf`.
- A `config.d/managed/` subdirectory, included **first**, for tool-generated fragments you do
  not hand-edit.
- A parallel `config.d.disabled/` directory that is **not** included. Move a fragment there to
  switch a whole set of hosts off without deleting it; move it back to re-enable.

Include paths are relative to `~/.ssh`. Because of first-match-wins, an earlier `Include` (and
an earlier line within a file) wins for any given parameter — so order your `Include` lines
deliberately, most-specific first.

A fragment is just an ordinary config file:

```text
# config.d/lab.conf
Host mesh-host
    HostName mesh-host.example.internal
    User alice
```

### Jumphosts and ProxyJump

When a host is not directly reachable — it sits behind a bastion or on an internal network —
you reach it by hopping through one or more intermediates. `ProxyJump` declares the chain
right in the config:

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

With this, `ssh target` transparently connects through `gateway`, then `jumphost`, then to
`target` — a two-hop chain, each hop reusing its own host block.

- **Inline, without config:** `ssh -J gateway,jumphost deploy@198.51.100.20`.
- `ProxyJump` is the modern, declarative choice and handles nested jumps automatically. Reach
  for a custom `ProxyCommand` only when you need a non-ssh tunnel — for example a corporate
  proxy binary — that `ProxyJump` cannot express.

### ssh-agent and passphrase-protected keys

A passphrase-protected key is decrypted once and held by an agent, so you are not re-prompted
on every connection. The setup differs per platform.

#### Linux

Most desktops start an agent via systemd user services or the login keyring. If
`ssh-add -l` reports "Could not open a connection to your authentication agent", start one and
add your key:

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

The Windows ssh-agent stores the added key in the registry, encrypted to your user account, so
it survives reboots and the passphrase is entered only once. (Microsoft recommends backing up
the key file and deleting it from disk afterwards, since the agent keeps a copy.)

#### WSL2

The simplest WSL2 setup treats the Linux distribution exactly like the Linux section above,
with its own keys, agent, and config.

If you already keep everything on the Windows side, you can instead **reuse the Windows agent
and config from WSL2** for a single source of truth. Two advanced options exist:

- **Call the Windows `ssh.exe` from WSL2.** Put a wrapper for `ssh` (and `ssh-add`, `scp`, …)
  ahead of the native Linux binaries so WSL2 uses the Windows OpenSSH client, which already
  reads `%USERPROFILE%\.ssh\config` and talks to the Windows Authentication Agent. You then
  maintain only the Windows config.
- **Bridge the agent socket** with a helper such as `npiperelay`, keeping the native WSL2 `ssh`
  but pointing `SSH_AUTH_SOCK` at the Windows agent.

Both are advanced and out of scope to walk through here, but two caveats matter:

- A shell **alias** (`alias ssh=ssh.exe`) only affects interactive shells. Programs that spawn
  `ssh` directly — including dune-admin's `command`-mode — do a `PATH` lookup for an executable
  named `ssh` and therefore ignore aliases. Use a real wrapper script or symlink on `PATH`
  (e.g. `~/.local/bin/ssh`) if non-interactive tools must pick it up.
- `ssh.exe` interprets paths the Windows way, so Linux-style arguments (`-i /home/...`,
  `ControlPath`, `IdentityFile`) will not resolve. Windows OpenSSH also has no ControlMaster
  multiplexing (see [Connection multiplexing (ControlMaster)](#connection-multiplexing-controlmaster)).

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

### Connection multiplexing (ControlMaster)

Multiplexing reuses one authenticated TCP connection for many sessions, making the second and
later connections to a host near-instant:

```text
Host *
    ControlMaster auto
    ControlPath ~/.ssh/cm-%C
    ControlPersist 60s
```

- `%C` hashes the connection parameters into the socket name, avoiding the roughly 104-character
  `ControlPath` length limit.
- `ControlPersist 60s` keeps the master connection alive briefly after the last session closes,
  so a quick follow-up reuses it.
- **Windows (native) does not support ControlMaster** — it has no Unix-domain sockets. WSL2,
  Linux, and macOS do. On native Windows, omit these options; each connection stands alone.

## Part 2 — dune-admin command-mode

### command-mode uses your SSH config

dune-admin's `command` SSH mode shells out to the OS `ssh` client, so it inherits your entire
`~/.ssh/config`: host aliases, `ProxyJump` chains, the agent, and `Include`d fragments. You
define the target host once in your config and point dune-admin at the alias.

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

- `-host` (env `SSH_HOST`) may be a plain `host:port` or a `~/.ssh/config` alias like
  `vm-target`; the alias resolves `HostName`, `User`, and the `ProxyJump` chain for you.
- The full env/flag reference (`SSH_HOST`, `SSH_MODE`, `SSH_EXTRA_OPTS`, key lookup) lives in
  the [README.md](README.md) config table — link, don't duplicate.
- **Windows note:** `command`-mode runs without ControlMaster multiplexing (see
  [Connection multiplexing (ControlMaster)](#connection-multiplexing-controlmaster)) — it is
  fully functional, just one connection per operation.

### Troubleshooting

- **See what ssh actually does:** `ssh -v vm-target` (add more `-v` for detail) prints config
  resolution and each ProxyJump hop.
- **"Bad permissions" / key ignored:** tighten modes with `chmod 700 ~/.ssh` and
  `chmod 600 ~/.ssh/config ~/.ssh/id_ed25519`.
- **Wrong `User` or `HostName` applied:** first-match-wins — a broad `Host *`, or an earlier
  `Include`, set the value before your specific block. Reorder so the specific block wins.
- **Repeated passphrase prompts:** the agent is not running or the key is not loaded. Check
  with `ssh-add -l`, then load it (see the agent section for your OS).
- **Auth fails at an intermediate hop:** ProxyJump authenticates at *each* hop. Ensure your key
  or agent reaches `gateway` and `jumphost`, not just `target`.
