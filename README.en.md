<p align="center">
  <img src="assets/termihomo-logo.svg" alt="Termihomo" width="180" />
</p>

<h1 align="center">Termihomo</h1>

<p align="center">
  <strong>Manage Mihomo from your terminal.</strong><br />
  A lightweight, cross-platform TUI and CLI for Mihomo.
</p>

<p align="center">
  <a href="README.md">简体中文</a> ·
  <a href="https://github.com/Nyakooo/termihomo/releases">Releases</a> ·
  <a href="docs/CLI.en.md">CLI reference</a> ·
  <a href="docs/ARCHITECTURE.en.md">Architecture</a>
</p>

<p align="center">
  <a href="https://github.com/Nyakooo/termihomo/actions/workflows/ci.yml"><img src="https://github.com/Nyakooo/termihomo/actions/workflows/ci.yml/badge.svg" alt="CI and Release" /></a>
  <a href="https://github.com/Nyakooo/termihomo/releases"><img src="https://img.shields.io/github/v/release/Nyakooo/termihomo" alt="Latest Release" /></a>
</p>

Termihomo brings day-to-day Mihomo management to the terminal. It combines the information layout of a desktop manager with the direct, lightweight, scriptable workflow of a command-line tool.

It is <strong>not a proxy core</strong>. Network traffic is handled by <a href="https://github.com/MetaCubeX/mihomo">Mihomo</a>; Termihomo manages the core lifecycle, profiles and subscriptions, proxies and policy groups, connections, DNS, and system network settings.

## Design goals

- **Terminal first**: a focused terminal interface without a desktop GUI dependency.
- **TUI + CLI**: interactive workflows and scriptable commands share the same management layer.
- **Lightweight by design**: manage Mihomo instead of reimplementing its proxy engine.
- **Cross-platform**: Linux, macOS, and Windows builds for AMD64 and ARM64.
- **Controlled changes**: managed configuration is validated before it is applied, and sensitive operations can require confirmation.

## Features

| Area | Capabilities |
| --- | --- |
| Core lifecycle | Download and verify Mihomo, start, stop, restart, inspect status, run in the background |
| Profiles | Import local YAML, add and refresh subscriptions, manage profile entries |
| Proxies | Browse proxies, select options in Selector groups, test latency, inspect delay history |
| Connections | Inspect active connections, process, destination, protocol, rule, outbound chain, and traffic |
| Traffic and status | Live upload/download rates, totals, memory usage, and uptime |
| DNS | DNS settings, query events, manual lookups, and results |
| Network | System proxy, proxy exclusions, TUN, stack mode, and outbound interface |
| Rules and logs | Inspect rules in evaluation order and follow Mihomo logs |
| Daily workflow | Chinese and English UI, configuration validation and backup, application self-update |

Availability of system proxy, startup integration, and TUN features varies by operating system and permissions.

## Interface

The TUI is organized into focused sections:

- **Overview** — core status, live traffic, memory, active profile, policy groups, and connections.
- **Proxies** — browse nodes and policy groups, select an option, and test latency.
- **Profile** — import, edit, refresh, and manage subscription or local profiles.
- **Connections** — inspect, filter, sort, and open connection details.
- **Tools** — general and network settings, DNS, core details, logs, and language.

Press <code>?</code> inside the application to view the current key bindings. The layout adapts to terminal size; a window of at least 80×24 is recommended.

### System tray

Enable **Tools → General → System tray** to show the tray, which is off by default. The tray stays running when the terminal interface closes. The TUI and CLI also work on servers without a desktop or tray host.

| Menu item | Action |
| --- | --- |
| Termihomo | Open the terminal interface if it is not already open |
| Current node | Open the node list or switch the existing interface to it |
| Routing mode | Cycle through rule, global, and direct |
| TUN | Toggle TUN using the platform's permission requirements |
| Latency | Test the current node immediately. The tray also tests the selected proxy node once per minute by default |
| Quit | Close the proxy, tray, and open Termihomo interfaces while keeping the tray preference |

The menu shows the latest node, routing mode, TUN state, and latency.

## Supported platforms

| Platform | Architectures | Installation |
| --- | --- | --- |
| Linux (kernel 5.3+) | AMD64, ARM64 | Shell installer / Releases |
| macOS | AMD64, ARM64 | Shell installer / Releases |
| Windows | AMD64, ARM64 | PowerShell installer / Releases |

## Installation

### Linux / macOS

Run the following command to install:

~~~sh
sh -c 'installer=$(curl -fsSL https://raw.githubusercontent.com/Nyakooo/termihomo/master/install.sh) && exec sh -c "$installer"'
~~~

Linux and macOS install to <code>~/.local/bin</code> by default. Add <code>~/.local/bin</code> to your shell's <code>PATH</code> if needed.

~~~sh
export PATH="$HOME/.local/bin:$PATH"
~~~

### Windows PowerShell

Run the following command in a PowerShell window.

~~~powershell
irm 'https://raw.githubusercontent.com/Nyakooo/termihomo/master/install.ps1' -ErrorAction Stop | iex
~~~

The default directory is <code>%LOCALAPPDATA%\Programs\Termihomo</code>. The installer adds it to the current user's <code>PATH</code>.

### Custom installation directory

Set <code>TERMIHOMO_INSTALL_DIR</code> before running the installer to override the default location.

### Manual installation

You can also download a build from <a href="https://github.com/Nyakooo/termihomo/releases">Releases</a>:

1. Choose the asset for your operating system and CPU architecture.
2. Download <code>SHA256SUMS</code> from the same release.
3. Verify the downloaded file with a local SHA-256 utility.
4. Extract or place the executable in a directory on your <code>PATH</code>.

The installers download the matching release and verify its SHA-256 checksum before installation.

## Uninstall

Run `termihomo uninstall` to close the tray and interface, stop the core, and remove startup registration, the executable, logs, and caches. Subscriptions, user configuration, and manual configuration backups are kept.

## Quick start

Launch the interactive interface:

~~~sh
termihomo
~~~

Install the Mihomo core before starting it: run <code>termihomo install</code>, or choose **Install / reinstall** from the TUI Core page. The installation flow downloads and verifies a supported build.

A typical first-run workflow:

1. Open **Profile** and add a subscription URL or import a local Mihomo YAML file.
2. Wait for the profile to pass validation.
3. Start Mihomo from **Core** or the Overview page.
4. Open **Proxies** and choose a node or policy-group option.
5. Enable the system proxy or TUN from **Network** if needed.

The default local mixed-proxy address is <code>127.0.0.1:7890</code>. LAN access is disabled by default. Enabling TUN or changing system network settings may require additional system privileges.

## CLI

Termihomo also provides a scriptable command-line interface:

~~~sh
termihomo help
termihomo status
termihomo profile list
termihomo proxy list
termihomo logs -f
~~~

Common commands:

| Command | Description |
| --- | --- |
| <code>termihomo</code> | Launch the interactive TUI |
| <code>termihomo install</code> | Download and verify the Mihomo core |
| <code>termihomo update</code> | Update the Termihomo executable |
| <code>termihomo uninstall</code> | Uninstall Termihomo and keep user configuration |
| <code>termihomo start</code> / <code>stop</code> | Start or stop the background core |
| <code>termihomo restart</code> / <code>reload</code> | Restart the core or reload its configuration |
| <code>termihomo status --json</code> | Print machine-readable status |
| <code>termihomo profile add URL</code> | Add a subscription |
| <code>termihomo profile import FILE</code> | Import a local YAML profile |
| <code>termihomo proxy list</code> | List proxies and policy groups |
| <code>termihomo proxy test NAME</code> | Test proxy latency |
| <code>termihomo rules --json</code> | Print rules as JSON |
| <code>termihomo config check</code> | Validate the managed configuration |

See the <a href="docs/CLI.en.md">CLI reference</a> for arguments, confirmation behavior, and examples.

## Documentation

| Document | Contents |
| --- | --- |
| [CLI reference](docs/CLI.en.md) | Commands, arguments, examples, and scripting |
| [Configuration and data](docs/CONFIGURATION.en.md) | Config files, data directories, logs, and environment variables |
| [Architecture](docs/ARCHITECTURE.en.md) | Module boundaries, runtime flow, and design principles |
| [Contributing](CONTRIBUTING.md) | Local development, tests, and contribution guidelines |

## Build from source

Use the Go toolchain version specified in <code>go.mod</code>.

~~~sh
git clone https://github.com/Nyakooo/termihomo.git
cd termihomo
go test ./...
mkdir -p bin
go build -trimpath -o bin/termihomo ./cmd/termihomo
~~~

Run the resulting binary:

~~~sh
./bin/termihomo
~~~

On Windows, use <code>termihomo.exe</code> as the output filename. See the <a href="docs/ARCHITECTURE.en.md">architecture guide</a> for module details.

## Acknowledgements

- <a href="https://github.com/MetaCubeX/mihomo">MetaCubeX / Mihomo</a> — proxy core and controller API.
- <a href="https://github.com/chen08209/FlClash">chen08209 / FlClash</a> — design reference for interaction layout and information organization.
- <a href="https://github.com/charmbracelet">Charmbracelet</a> — terminal UI components.

Termihomo is an independent management tool and is not affiliated with or officially endorsed by these projects.

## Project status

Termihomo is under active development. Features, commands, and configuration formats may change between versions. Before using Termihomo, please check the Release Notes for the corresponding version.

---

<p align="center">Built for the terminal.</p>
