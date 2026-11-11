# Termihomo

[简体中文](README.md)

Termihomo manages Mihomo from a terminal UI or a scriptable CLI. The managed Mihomo process keeps serving proxy traffic when you close the UI.

V0.1 targets Linux AMD64 and ARM64. It requires Linux 5.3 or later for safe process management. Build with Go 1.26.

```sh
go build -o termihomo ./cmd/termihomo
./termihomo install
./termihomo profile import /path/to/config.yaml
./termihomo start
./termihomo
```

In the TUI, press `Tab` or `1`–`7` to switch pages, `?` for help, and `q` to exit. The Core page manages Mihomo, the Groups page switches Selector options, and the Traffic and Logs pages show live data.

English and Simplified Chinese are available in both interfaces. Termihomo follows the terminal locale by default. Use `./termihomo --lang en` or `./termihomo --lang zh-CN` for a single invocation, or set `TERMIHOMO_LANG`. Put `--lang` before the command. Command names and `status --json` field names and state values are stable across languages. Mihomo log messages and imported proxy names retain their original text.

```sh
./termihomo status --json
./termihomo proxy list
./termihomo proxy select 'Group' 'Proxy'
./termihomo logs -f
./termihomo stop --yes
```

Read the `proxy` field in `status --json` for the current HTTP/SOCKS address. Termihomo tries `127.0.0.1:7890` first and selects an available loopback port if it is in use. Exit code `0` means success, `1` means the operation failed, and `2` means invalid arguments or missing confirmation for a non-interactive operation.

For subscriptions, use `./termihomo profile add -` to read the URL from standard input and keep credentials out of command arguments. Run `./termihomo profile update` to refresh it. This release manages one profile and accepts full Mihomo YAML files. Termihomo binds the controller and proxy to loopback and removes extra listeners and TUN settings when generating the managed configuration.

TUI shortcuts and the refresh interval can be set in `${XDG_CONFIG_HOME:-$HOME/.config}/termihomo/settings.yaml`. See the [settings example](README.md). Run `go test ./...` and `go vet ./...` for Go checks. After building, run `python3 test/smoke.py ./termihomo` for the end-to-end checks. This requires network access and `curl`.
