<p align="center">
  <img src="assets/termihomo-logo.svg" alt="Termihomo logo" width="240" />
</p>

<h1 align="center">Termihomo</h1>

<p align="center"><a href="README.md">简体中文</a> · <a href="https://github.com/Nyakooo/termihomo/releases">Downloads</a></p>

Manage Mihomo from your terminal. Import subscriptions, switch nodes, test latency, and inspect active connections. On desktop, Mihomo can keep running after you close the UI.

The dashboard and main interactions draw on [FlClash](https://github.com/chen08209/FlClash). Thanks to chen08209 and the FlClash contributors for the design, and to [MetaCubeX](https://github.com/MetaCubeX/mihomo/releases) for the proxy core.

## Install

| Platform | Architecture | Default install location |
| --- | --- | --- |
| Linux (kernel 5.3+) / macOS | AMD64, ARM64 | `~/.local/bin` |
| Windows | AMD64, ARM64 | `%LOCALAPPDATA%\Programs\Termihomo` |
| Termux (Android 8.0+) | ARM64 | `$PREFIX/bin` |

**Linux, macOS, Termux**

```sh
installer=$(curl -fsSL https://raw.githubusercontent.com/Nyakooo/termihomo/master/install.sh) && printf '%s\n' "$installer" | sh
```

**Windows PowerShell**

```powershell
& ([scriptblock]::Create((Invoke-RestMethod 'https://raw.githubusercontent.com/Nyakooo/termihomo/master/install.ps1' -ErrorAction Stop)))
```

For a private repository, run `gh auth login` and use the command for your shell.

```sh
installer=$(gh api -H 'Accept: application/vnd.github.raw+json' repos/Nyakooo/termihomo/contents/install.sh) && printf '%s\n' "$installer" | sh
```

```powershell
$script = gh api -H 'Accept: application/vnd.github.raw+json' repos/Nyakooo/termihomo/contents/install.ps1
if ($LASTEXITCODE -ne 0) { throw 'Could not download install.ps1.' }
& ([scriptblock]::Create(($script -join "`n")))
```

The installers verify `SHA256SUMS`. On Linux / macOS, run `export PATH="$HOME/.local/bin:$PATH"` if your shell cannot find the command. You can also download a binary from [Releases](https://github.com/Nyakooo/termihomo/releases).

## Use

| Command | Purpose |
| --- | --- |
| `termihomo` | Open the TUI and download Mihomo on first use |
| `termihomo update` | Update the Termihomo executable |
| `termihomo status` | Show the core and proxy status |
| `termihomo start` | Start the background core |
| `termihomo stop` | Stop the background core |
| `termihomo reload` | Apply the active profile and saved settings in the current process. Port changes require a restart |
| `termihomo restart` | Stop and start the core, interrupting active connections |
| `termihomo tun start` / `termihomo tun stop` | Enable or disable TUN on the running core. System privileges are required to enable it |

For private releases, set `GH_TOKEN` with repository read access before running `update`. The proxy defaults to `127.0.0.1:7890`. Termux provides a local proxy. Device-wide VPN requires an Android client.
