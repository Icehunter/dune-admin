# AMP Control Plane

The `amp` control plane targets CubeCoders AMP installations. Selected via `control: amp` in config.

## Topology

```
host (e.g. Ubuntu VM)
 └── AMP web panel (port 8080)
      └── podman container "AMP_<instance>"  (cubecoders/ampbase)
           ├── ampinstmgr (lifecycle)
           ├── RabbitMQ broker (admin + game vhosts)
           ├── Postgres
           └── 1..N DuneSandboxServer-Linux-Shipping processes (one per partition)
```

`dune-admin` runs **on the host**. Uses `localExecutor` for shell and `ampExecutor` to write INI files
as the AMP user.

## Config Keys

```yaml
control: amp
amp_instance:   DuneAwakening01
amp_container:  AMP_DuneAwakening01       # default: AMP_<instance>
amp_container_runtime: podman             # podman (default) | docker
amp_container_stop_timeout: 60            # restart graceful-stop seconds before SIGKILL (default 60)
amp_update_auto_restart: true             # auto-restart container after an update finishes (default true)
amp_user:       amp
amp_log_path:   /AMP/duneawakening/logs   # in-container log dir
amp_api_user:   admin                     # AMP panel login — enables gameplay-settings writes
amp_api_pass:   yourpassword
amp_api_port:   8081                       # instance ADS API port (default 8081)
director_url:   http://127.0.0.1:11717    # optional — enables /director/ proxy
broker_exec_prefix: "sudo -i -u amp podman exec AMP_DuneAwakening01"
server_ini_dir: /home/amp/.ampdata/instances/DuneAwakening01/duneawakening/server/state
db_host: 127.0.0.1
db_port: 15432
```

## Sudoers

```
dune-admin ALL=(amp) NOPASSWD: /usr/bin/ampinstmgr, /usr/bin/podman, /usr/bin/tee
```

Use `/usr/bin/docker` instead of `/usr/bin/podman` when `amp_container_runtime: docker`. Narrow
`tee` to specific INI paths under `server_ini_dir` in production.

## Provider Behaviour

| Method | Implementation |
| --- | --- |
| `GetStatus` | Lists `DuneSandboxServer-Linux-Shipping` host processes; reports container DB phase |
| `ExecCommand` | start/stop: `ampinstmgr -s/-q <amp_instance>`. restart (container): `<runtime> restart -t <amp_container_stop_timeout> <container>` — `ampinstmgr` does NOT reap game procs; container restart is the only way to cycle them (generous stop timeout so it doesn't wedge on SIGKILL). update: AMP Web API `Core/UpdateApplication` (SteamCMD), then a background watcher polls `Core/GetStatus` RunningTasks and restarts the container when the update finishes (unless `amp_update_auto_restart: false`) |
| `writeServerSettings` | AMP Web API `Core/Login` + `Core/SetConfig` (node `Meta.GenericModule.<FieldName>`) via in-container curl; needs `amp_api_*` |
| `ListProcesses` | Host `ps` for game-server processes, decorated with map/port/partition |
| `ListLogSources` | `<runtime> exec <container> ls <amp_log_path>` |
| `StreamLog` | `<runtime> exec <container> tail -F <amp_log_path>/<name>` |
| `CaptureJWT` | Extracts `ServiceAuthToken` from game-server process args on host |
| `ListExchanges` / `EnsureCaptureUser` | `rabbitmqctl` via `broker_exec_prefix` |
| `DiscoverIniDir` | Returns `server_ini_dir` |
| `ReadDefaultINI` | `<runtime> exec <container> find / -name <file>` then `cat` |

## Key Behaviours

- **Server settings go through AMP Web API, not INI writes.** AMP regenerates `UserEngine.ini` /
  `UserGame.ini` on every start; direct file edits are clobbered. Non-AMP control planes write
  files directly via `ampExecutor.WriteFile`.
- **`ampControl.startEnsureCaptureUserLoop`** re-applies the `dune_cap` user+permissions every 15s
  so capture survives broker restarts.
- **Dev box**: `ssh amp@192.168.0.59` — see memory `project_amp_dev_box.md` for INI paths.
