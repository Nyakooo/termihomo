# Configuration and Data

This document describes what Termihomo stores locally, where its default directories are located, and which files are intended for manual changes.

## Directory layout

Termihomo keeps configuration, runtime data, and logs in separate locations.

| Platform | Config directory | Data directory | Log directory |
| --- | --- | --- | --- |
| Linux | `~/.config/termihomo` | `~/.local/share/termihomo` | `~/.local/state/termihomo/logs` |
| macOS | `~/Library/Application Support/termihomo/config` | `~/Library/Application Support/termihomo/data` | `~/Library/Application Support/termihomo/state/logs` |
| Windows | `%LOCALAPPDATA%\termihomo\config` | `%LOCALAPPDATA%\termihomo\data` | `%LOCALAPPDATA%\termihomo\state\logs` |

Linux supports the XDG Base Directory environment variables. When set to absolute paths, Termihomo creates a `termihomo` subdirectory inside each location:

- `XDG_CONFIG_HOME`
- `XDG_DATA_HOME`
- `XDG_STATE_HOME`

## Important files

### Config directory

| Path | Purpose |
| --- | --- |
| `profile.json` | Active profile and its metadata |
| `profiles/` | Saved profile library |
| `overrides.yaml` | Configuration overrides managed by Termihomo |
| `settings.yaml` | TUI refresh interval and key bindings |
| `language` | Saved interface language |

You can use `settings.yaml` to change the refresh interval and selected key bindings. For example:

~~~yaml
refresh-seconds: 3
keys:
  quit: q
  help: "?"
  refresh: r
  search: /
  sort: s
  log-level: v
~~~

Key bindings must not conflict with reserved interface keys. Invalid settings are reported when the TUI starts.

### General toggles

Press Enter on a Boolean setting in **Tools → General** to toggle it.

| Setting | Behavior |
| --- | --- |
| Auto-run | Start the proxy when the Termihomo interface opens |
| Start on boot | Start the managed core when the computer boots. Registering the pre-login task on Windows requires administrator privileges |
| Close connections on node change | Close all active Mihomo connections after a manual node change. Off by default |
| System tray | Keep a tray item running when a tray host is available. Off by default |

### Data directory

| Path | Purpose |
| --- | --- |
| `instance.json` | Runtime state for the managed core |
| `mihomo/config.yaml` | Runtime configuration generated from the active profile and application settings |
| `mihomo/config.yaml.manual.bak` | Manual backup created by `termihomo config backup` and kept during uninstall |
| `cores/` | Installed Mihomo core versions |

### Log directory

| Path | Purpose |
| --- | --- |
| `termihomo.log` | Termihomo application log |

Mihomo runtime logs can be viewed from the TUI Logs page or with `termihomo logs`.

## Environment variables

| Variable | Purpose |
| --- | --- |
| `XDG_CONFIG_HOME` | Parent directory for Linux configuration |
| `XDG_DATA_HOME` | Parent directory for Linux application data |
| `XDG_STATE_HOME` | Parent directory for Linux state and logs |
| `TERMIHOMO_LANG` | Default language; accepted values are `en` and `zh-CN` |
| `TERMIHOMO_INSTALL_DIR` | Override the installer's default destination |

The `--lang` command-line option selects a language for the current launch. A language selected in the TUI is saved in the config directory.

## How configuration is applied

Termihomo combines the active profile with application overrides to generate a managed configuration:

1. Import a local Mihomo YAML file or download a subscription profile.
2. Save the active profile.
3. Apply Termihomo settings to the corresponding configuration fields.
4. Ask Mihomo to validate the generated result.
5. Write and start or reload the configuration only after validation succeeds.

For this reason, `mihomo/config.yaml` is a generated runtime file and should not be edited directly. It may be regenerated when the core starts, reloads, or settings change.

Use the TUI settings pages to adjust general, DNS, and network options. Manage profiles through the Profile page or CLI.

## Proxy defaults

- Default mixed-proxy listener: `127.0.0.1:7890`
- LAN access is disabled by default.
- Termihomo manages the runtime configuration. Avoid having another tool overwrite the same managed config.
- System proxy and TUN settings affect system networking; consider permissions and network impact before enabling them.

## Security and backups

Configuration directories are created as private directories accessible to the current user. Do not loosen their permissions casually or commit the configuration directory to a public repository.

Keep in mind:

- Subscription URLs may contain account tokens or other access credentials.
- Profiles may contain proxy passwords, keys, domains, and personal network details.
- Inspect and redact logs before sharing them; they may contain subscription URLs, server addresses, or other sensitive data.
- `termihomo config backup` backs up the generated runtime configuration, not the complete application state. For a full migration, stop the core and securely back up the config directory and any required data.

## Installation directory

The installer places the Termihomo executable in these default locations:

- Linux / macOS: `~/.local/bin/termihomo`
- Windows: `%LOCALAPPDATA%\Programs\Termihomo\termihomo.exe`

Set `TERMIHOMO_INSTALL_DIR` to override the installer's default target. This changes only the executable location; it does not change application config, data, or log directories.
