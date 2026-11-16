<p align="center">
  <img src="assets/termihomo-logo.jpg" alt="Termihomo logo" width="240" />
</p>

<h1 align="center">Termihomo</h1>

<p align="center"><a href="README.md">简体中文</a></p>

Termihomo manages Mihomo from a terminal UI or a scriptable CLI. The managed Mihomo process keeps serving proxy traffic when you close the UI.

The dashboard, page layout, and key interactions draw on [FlClash](https://github.com/chen08209/FlClash). Thanks to chen08209 and the FlClash contributors for their design, and to [MetaCubeX](https://github.com/MetaCubeX/mihomo) for the proxy core.

Termihomo targets Linux, macOS, and Windows on AMD64 and ARM64, and Termux on Android ARM64. Build with Go 1.26. Desktop Linux requires version 5.3 or later.

Download the build for your platform from [GitHub Releases](https://github.com/Nyakooo/termihomo/releases) and verify it against `SHA256SUMS` on the same page. On Linux and macOS, extract the archive and run `./termihomo`. On Windows, run the downloaded `.exe`. Termux installation is covered below.

To build from source:

```sh
go build -o bin/termihomo ./cmd/termihomo
./bin/termihomo install
./bin/termihomo profile import /path/to/config.yaml
./bin/termihomo start
./bin/termihomo
```

On Windows, build with `go build -o bin/termihomo.exe ./cmd/termihomo`. Use `.\bin\termihomo.exe` in PowerShell or Command Prompt and `./bin/termihomo.exe` in Git Bash. On MinTTY, the app uses the `winpty` adapter shipped with Git for Windows. Git Bash also works from Windows Terminal.

### Android Termux

The Android ARM64 build targets Termux on Android 8.0 or later. Extract the release archive, run `install -m 700 ./termihomo-android-arm64 "$PREFIX/bin/termihomo"` in Termux, then run `termihomo`.

To build from source, open the project directory in Termux:

```sh
pkg install golang clang ca-certificates
CGO_ENABLED=1 go build -trimpath -ldflags="-extldflags=-Wl,-z,max-page-size=16384,-z,common-page-size=16384" \
  -o "$PREFIX/bin/termihomo" ./cmd/termihomo
termihomo
```

Termihomo downloads and verifies the Android Mihomo core. Use the TUI or CLI to manage subscriptions, switch nodes, and test latency. The local HTTP/SOCKS proxy defaults to `127.0.0.1:7890`. The TUI requires at least 64 columns and 24 rows. The clipboard button uses the optional Termux:API integration.

Device-wide VPN, Android system proxy integration, and startup at boot require separate Android integration. Android can reclaim background Termux processes. Settings are saved under `~/.config/termihomo` by default.

Press `Tab` or `1`–`5` to open pages. In Tools, click a tab or use `[` / `]`. Press `?` for help and `q` to exit.

Press `a` on the Dashboard to toggle the proxy listener. The core remains available for browsing, testing, and changing nodes while the listener is off. Tools → Core and the CLI `stop` command terminate the core process.

The Proxies page follows the configuration's group and member order. Click a group tab or press `[` / `]` to switch groups. Use the arrow keys, `hjkl`, or a mouse click to focus a node. Press `Enter` to apply the focused node in a Selector group, `t` to test it, `f` to test all nodes, `s` to toggle delay sorting, and `e` for details.

English and Simplified Chinese are available in both interfaces. Termihomo defaults to English. Choose Tools → Language to switch languages and save your preference. Use `./bin/termihomo --lang en` or `./bin/termihomo --lang zh-CN` for a single invocation, or set `TERMIHOMO_LANG`. Put `--lang` before the command. Command names and `status --json` field names and state values are stable across languages. Mihomo log messages and imported proxy names retain their original text.

The Requests table shows processes, destinations, protocols, rules, exits, rates, totals, and durations. Click the filter chips to toggle multiple protocols or rules. You can also open either filter menu with `Ctrl+P` / `Ctrl+T`, toggle options with Enter and close the menu with Esc. Logs are under Tools. The table displays active connections. Press `c` to group rows. Use `h` / `l` to scroll columns in narrow terminals. Enable the system proxy manually with `p` on the Dashboard.

Tools → General configures the proxy port, startup behavior, connection handling when switching nodes, and delay testing. Tools → DNS shows live query records with filters, manual lookup, and cache clearing. Press Enter to edit, `d` to restore the inherited value, and `Ctrl+R` to apply core settings and restart.

```sh
./bin/termihomo status --json
./bin/termihomo proxy list
./bin/termihomo proxy select 'Group' 'Proxy'
./bin/termihomo logs -f
./bin/termihomo stop --yes
```

Read the `proxy` field in `status --json` for the current HTTP/SOCKS address. Termihomo defaults to `127.0.0.1:7890`. If that port is in use, startup reports an error. Set a different port under Tools → General. Exit code `0` means success, `1` means the operation failed, and `2` means invalid arguments or missing confirmation for a non-interactive operation.

For subscriptions, use `./bin/termihomo profile add -` to read the URL from standard input and keep credentials out of command arguments. Run `./bin/termihomo profile update` to refresh it. The Profiles page manages multiple subscriptions and local Mihomo YAML files. Termihomo binds the controller and proxy to loopback and removes extra listeners and TUN settings when generating the managed configuration.

Tools → General includes two startup settings. Auto-run starts Mihomo when the UI opens. Start on boot registers an operating-system startup job through systemd, launchd, or Windows Task Scheduler. macOS prompts for administrator authorization and Windows requires an elevated terminal. TUN also requires an elevated Mihomo process on macOS and Windows. On macOS, a `sudo` session uses the root account's configuration directories.

The default configuration folders are `~/.config/termihomo` on Linux, `~/Library/Application Support/termihomo/config` on macOS, and `%LOCALAPPDATA%/termihomo/config` on Windows. An absolute `XDG_CONFIG_HOME` overrides the default. See the [settings example](README.md). Run `go test ./...` and `go vet ./...` for Go checks. After building, run `TERMIHOMO_SMOKE_BINARY=../bin/termihomo go test -count=1 ./test` from the repository root to verify real proxy traffic, rollback, and process lifecycle. The release test requires network access.

See the [architecture notes](docs/ARCHITECTURE.md) and the [Mihomo integration record](docs/TECHNICAL_RESEARCH.md).
