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

### Routing rules

Open **Profile → Routing rules** to manage custom rules for the active profile without editing the generated `mihomo/config.yaml`.

- `a`: choose `DOMAIN`, `DOMAIN-SUFFIX`, `IP-CIDR` (IPv4), or `DST-PORT`, enter the value, then select a configured proxy group, `DIRECT`, or `REJECT`.
- `e` / Enter edits, `d` deletes, Space enables/disables, and `Alt+↑/↓` changes custom rule priority. Buttons and lists support the mouse.
- Original rules remain read-only and in their original order. Use `c` to copy a supported type into a custom rule, preserving the IP-CIDR `no-resolve` option. Other unsupported options are explicitly rejected. Duplicate custom matchers are rejected.
- `Ctrl+S` (**Save and apply**) validates the complete configuration before saving and hot applying it. Failures show the error and keep your draft. Normal application failures restore the previous configuration and runtime settings. If a communication failure leaves the running state unconfirmed, the error asks you to restart the core; it does not automatically close all connections.
- `Ctrl+R` resets the draft to the active profile's saved rules. Stale drafts cannot overwrite a switched or updated profile.

Rules are stored in each profile's `custom_rules` field and prepended to the original rules in list order. Disabled rules remain saved but are omitted from the generated rules. Restarting, subscription updates, and switching profiles retain each profile's own rules. If an update removes a referenced proxy group, the update fails and keeps the previous profile; adjust the custom rule target first.

For example, to route a service through a proxy group, add a `DOMAIN` rule for `mail.example.com`, select the desired group from the target list, and select a node in that group on the Proxies page. Do not include a port in domain values. Save and apply, choose **Rule mode** on the overview, and reconnect the affected application or service. Rules participate in routing only in Rule mode; existing connections may keep their previous route.

### General toggles

Press Enter on a Boolean setting in **Tools → General** to toggle it.

| Setting | Behavior |
| --- | --- |
| Auto-run | Start the proxy when the Termihomo interface opens |
| Start on boot | Start the managed core when the computer boots. Registering the pre-login task on Windows requires administrator privileges |
| Close connections on node change | Close all active Mihomo connections after a manual node change. Off by default |
| System tray | Keep a tray item running when a tray host is available. Off by default |

System proxy, TUN, and LAN proxy switches are saved and reapplied on boot, auto-run, and resume. Stopping or pausing the proxy preserves these preferences. Core restarts and configuration reloads also retain the LAN proxy state.

If desktop system proxy settings are unavailable before login, restoration is retried at desktop login even when the system tray is disabled. Login restoration waits up to two minutes for a busy instance lock, core readiness, or network readiness, without requesting TUN authorization. At boot, individual network restoration failures are logged while the started core remains running. TUN still requires network administration privileges, and LAN proxy activation requires a private IPv4 network. Restoration failures are reported without clearing saved switches.

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
