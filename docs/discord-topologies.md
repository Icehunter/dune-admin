# Discord Topologies Supported by `dune-admin`

This document describes the Discord topology model supported by `dune-admin`.

## Core Rules

These rules apply to every supported scenario:

- Each game server links to **exactly one Discord guild**.
- A Discord guild can serve **multiple game servers**.
- One bot token serves every guild.
- Commands route by the channel they are invoked in:
  - command channel → game server → linked guild for auth and roles
- Each linked game server has its own:
  - announcement channel
  - status channel
- Players have a distinct character per server.
  - `/register`, `/mystats`, and similar commands resolve against the server tied to the channel where the command is run.

## Supported Scenarios

### 1. Single Server, Single Guild

The common case:

- 1 `dune-admin` instance
- 1 game server
- 1 Discord guild
- 1 announcement/status channel pair

This is backward-compatible with the legacy single-guild install. The initial configuration can be seeded automatically from the old `DiscordGuildID`.

### 2. Multiple Servers, One Shared Guild

A single Discord guild can serve multiple game servers.

Example:

- 1 `dune-admin` instance
- N game servers, such as 3 or 20
- 1 Discord guild
- N announcement/status channel pairs

Each game server gets its own announcement and status channels inside the shared guild.

Command routing is based on the channel:

- `/mystats` in server 2's channel resolves to the player's server 2 character.
- `/mystats` in server 5's channel resolves to the player's server 5 character.

This covers the example of 1 `dune-admin` instance, 3 game servers, 1 guild, and 3 channel pairs. The same pattern scales to more servers, such as 20.

### 3. Multiple Servers, Multiple Guilds

Each game server can have its own dedicated Discord guild.

Example:

- 1 `dune-admin` instance
- N game servers
- N Discord guilds
- 1 announcement/status channel pair per guild

This is useful when each game server represents a separate community with its own Discord server.

### 4. Multiple Servers, Mixed Guild Grouping

Game servers can be distributed across several guilds, where some guilds serve one server and others serve multiple servers.

Example:

- 20 game servers spread across 4 guilds
- Guild A serves servers 1-5
- Guild B serves servers 6-10
- Other guilds serve the remaining servers
- Each server still has its own announcement/status channel pair within its linked guild

This is the general case. The shared-guild and one-guild-per-server scenarios are just endpoints of this model.

### 5. Servers With No Discord Link

A game server can have no Discord guild or channels configured.

In that case, the server:

- does not post announcements
- does not post status updates
- does not accept Discord commands

Mixed installs are supported. Some servers can be linked to Discord while others remain unlinked.

## Unsupported Scenarios

The following are not supported:

- A single game server linked to more than one Discord guild.
- One shared character across multiple servers.

Characters are scoped per `(player, server)`. A player can have a different character on each server and can delete or transfer characters between servers as supported by the application.

## Practical Limits

- The bot must be a member of every guild it serves.
- The bot needs the required permissions in each announcement and status channel:
  - View Channel
  - Send Messages
  - Embed Links
- Status loops and command registration run per linked server and guild configuration.

For example, 20 linked game servers means:

- 20 status loops
- 20 announcement/status channel pairs
- 1 shared bot token

## Summary

The supported topology spectrum is:

```text
1 server / 1 guild
→ N servers / 1 guild
→ N servers / N guilds
→ N servers distributed across several guilds
```

The key invariant is that any single game server maps to exactly one Discord guild, and command routing is always determined by the channel where the command is invoked.
