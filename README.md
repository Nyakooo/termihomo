<p align="center">
  <img src="assets/termihomo-logo.svg" alt="Termihomo logo" width="240" />
</p>

<h1 align="center">Termihomo</h1>

<p align="center"><a href="README.en.md">English</a> · <a href="https://github.com/Nyakooo/termihomo/releases">下载</a></p>

用终端管理 Mihomo。支持订阅、节点切换、测速和实时连接查看。桌面端退出界面后，Mihomo 可继续在后台运行。

界面布局和主要交互参考了 [FlClash](https://github.com/chen08209/FlClash)。感谢 chen08209 和 FlClash 贡献者，也感谢 [MetaCubeX](https://github.com/MetaCubeX/mihomo/releases) 提供代理内核。

## 安装

| 平台 | 架构 | 默认安装目录 |
| --- | --- | --- |
| Linux（内核 5.3+） / macOS | AMD64、ARM64 | `~/.local/bin` |
| Windows | AMD64、ARM64 | `%LOCALAPPDATA%\Programs\Termihomo` |
| Termux（Android 8.0+） | ARM64 | `$PREFIX/bin` |

**Linux、macOS、Termux**

```sh
installer=$(curl -fsSL https://raw.githubusercontent.com/Nyakooo/termihomo/master/install.sh) && printf '%s\n' "$installer" | sh
```

**Windows PowerShell**

```powershell
& ([scriptblock]::Create((Invoke-RestMethod 'https://raw.githubusercontent.com/Nyakooo/termihomo/master/install.ps1' -ErrorAction Stop)))
```

私有仓库可用已登录的 GitHub CLI 安装。先运行 `gh auth login`，再执行对应命令。

```sh
installer=$(gh api -H 'Accept: application/vnd.github.raw+json' repos/Nyakooo/termihomo/contents/install.sh) && printf '%s\n' "$installer" | sh
```

```powershell
$script = gh api -H 'Accept: application/vnd.github.raw+json' repos/Nyakooo/termihomo/contents/install.ps1
if ($LASTEXITCODE -ne 0) { throw 'Could not download install.ps1.' }
& ([scriptblock]::Create(($script -join "`n")))
```

安装脚本会校验 `SHA256SUMS`。Linux / macOS 如找不到命令，可在当前终端执行 `export PATH="$HOME/.local/bin:$PATH"`。也可以从 [Releases](https://github.com/Nyakooo/termihomo/releases) 手动下载。

## 使用

| 命令 | 用途 |
| --- | --- |
| `termihomo` | 打开终端界面，首次使用会下载 Mihomo |
| `termihomo update` | 更新 Termihomo 本体 |
| `termihomo status` | 查看内核和代理状态 |
| `termihomo start` | 启动后台内核 |
| `termihomo stop` | 停止后台内核 |
| `termihomo reload` | 在当前进程中应用当前配置和已保存的设置，端口变更需重启 |
| `termihomo restart` | 停止并重新启动内核，现有连接会断开 |
| `termihomo tun start` / `termihomo tun stop` | 为运行中的内核开启或关闭 TUN，开启时需要系统权限 |

私有仓库的 `update` 需要设置有仓库读取权限的 `GH_TOKEN`。代理默认监听 `127.0.0.1:7890`。Termux 提供本地代理，全机 VPN 需要 Android 客户端。
