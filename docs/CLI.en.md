# CLI Reference

The Termihomo CLI and TUI share the same management layer. The CLI is useful for remote administration, scripts, and workflows that do not need an interactive interface.

List commands supported by the current build:

```sh
termihomo help
```

## Global options

| Option | Description |
| --- | --- |
| `--lang en` | Use English output and interface |
| `--lang zh-CN` | Use Simplified Chinese output and interface |
| `--help`, `-h` | Show help |

Place the language option before the command:

```sh
termihomo --lang en status
termihomo --lang zh-CN
```

## Application and core

| Command | Description |
| --- | --- |
| `termihomo` | Launch the interactive TUI |
| `termihomo init` | Initialize application config, data, and log directories |
| `termihomo install` | Download and verify a supported Mihomo core |
| `termihomo update` | Update the Termihomo executable |
| `termihomo uninstall` | Remove the executable and runtime files while keeping user configuration |
| `termihomo version` | Print the version and current platform |

### Uninstall

`termihomo uninstall` closes the tray and terminal interface, restores system proxy settings owned by Termihomo, removes startup registration, and stops the managed Mihomo process. It removes the current executable, core, logs, and caches while keeping subscriptions, profiles, application settings, and manual backups made with `termihomo config backup`. On Windows, the executable is removed after the command exits.

### Core lifecycle

```sh
termihomo start
termihomo status
termihomo reload
termihomo restart
termihomo stop
```

- `start`: start the Mihomo process managed by Termihomo. Also start the tray if enabled and supported by the current desktop.
- `status`: inspect process and Controller state.
- `reload`: reload configuration in the current process. Changes to startup-level options may require a restart.
- `restart`: stop and start the core again. Existing connections will be interrupted. Also start the tray if enabled and supported; an existing tray is reused.
- `stop`: stop the managed core process and restore system proxy settings owned by Termihomo. Keep the tray and terminal interface open; the tray refreshes to show the stopped state.

Status can be emitted as JSON:

```sh
termihomo status --json
```

### TUN

```sh
termihomo tun start
termihomo tun stop
```

Enabling TUN usually requires network-administration privileges. Confirm that the operating system and environment meet Mihomo's TUN requirements first.

## Profiles

| Command | Description |
| --- | --- |
| `termihomo profile list` | Show the active profile |
| `termihomo profile add URL` | Add and apply a subscription |
| `termihomo profile import FILE` | Import and apply a local YAML profile |
| `termihomo profile update` | Refresh the active subscription |

Examples:

```sh
termihomo profile add "https://example.com/your-subscription"
termihomo profile import ./config.yaml
termihomo profile update
```

Subscription URLs may contain personal credentials. Do not put real URLs in public scripts, Issues, or logs.

For non-interactive execution, append `--yes` to supported mutation commands to skip confirmation prompts:

```sh
termihomo profile update --yes
termihomo profile import ./config.yaml --yes
```

If an operation replaces the active profile, back up important configuration first.

## Proxies and policy groups

| Command | Description |
| --- | --- |
| `termihomo proxy list` | List proxies and policy groups |
| `termihomo proxy select GROUP OPTION` | Select an option in a Selector group |
| `termihomo proxy test NAME [URL]` | Test the delay of a proxy |

Examples:

```sh
termihomo proxy list
termihomo proxy select "Proxy" "Hong Kong"
termihomo proxy test "Hong Kong"
```

Group and option names must match the active configuration. Direct selection through this command is supported for Selector groups.

## Rules and logs

```sh
termihomo rules
termihomo rules --json
termihomo logs
termihomo logs -f
```

- `rules` lists active rules in evaluation order.
- `rules --json` prints JSON for scripts.
- `logs` shows recent Mihomo logs.
- `logs -f` follows new log entries.

## Configuration validation and backup

```sh
termihomo config check
termihomo config backup
```

- `config check` asks Mihomo to validate the current managed configuration.
- `config backup` backs up the generated runtime configuration and prints the backup path.

This backup covers the generated runtime config; it is not an export of every profile and application data file. See [Configuration and data](CONFIGURATION.en.md) for storage locations.

## Exit status

CLI commands return exit code `0` on success and a non-zero code for invalid arguments, execution failures, or validation errors. Scripts can use the process exit code to detect failures.
