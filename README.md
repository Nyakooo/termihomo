# Termihomo

[English](README.en.md)

Termihomo 是 Mihomo 的终端管理工具。它提供 TUI 和可脚本化 CLI。退出 TUI 后，托管的 Mihomo 继续在后台代理。

仪表盘、页面布局和主要交互参考了 [FlClash](https://github.com/chen08209/FlClash)。感谢 chen08209 和 FlClash 贡献者的设计，也感谢 [MetaCubeX](https://github.com/MetaCubeX/mihomo) 提供代理内核。

支持 Linux、macOS 和 Windows 的 AMD64 / ARM64，以及 Android ARM64 上的 Termux。使用 Go 1.26 构建。桌面 Linux 需要 5.3 或更新版本。

从 [GitHub Releases](https://github.com/Nyakooo/termihomo/releases) 下载对应平台的构建，使用同一页面的 `SHA256SUMS` 校验。Linux 和 macOS 的压缩包解压后运行 `./termihomo`，Windows 下载后运行对应的 `.exe`。Termux 安装方法见下文。

也可以从源码构建。

```sh
go build -o bin/termihomo ./cmd/termihomo
./bin/termihomo install
./bin/termihomo profile import /path/to/config.yaml
./bin/termihomo start
./bin/termihomo
```

Windows 使用 `go build -o bin/termihomo.exe ./cmd/termihomo` 构建。PowerShell 和 CMD 运行 `.\bin\termihomo.exe`，Git Bash 运行 `./bin/termihomo.exe`。Git Bash 的 MinTTY 会通过 Git for Windows 自带的 `winpty` 启动交互界面，也可在 Windows Terminal 内启动 Git Bash。

### Android Termux

Termihomo 提供 Android ARM64 构建，面向 Android 8.0 及更新版本的 Termux。解压发布包后，在 Termux 中执行 `install -m 700 ./termihomo-android-arm64 "$PREFIX/bin/termihomo"`，然后运行 `termihomo`。

也可以在 Termux 中进入项目目录后从源码构建。

```sh
pkg install golang clang ca-certificates
CGO_ENABLED=1 go build -trimpath -ldflags="-extldflags=-Wl,-z,max-page-size=16384,-z,common-page-size=16384" \
  -o "$PREFIX/bin/termihomo" ./cmd/termihomo
termihomo
```

首次打开界面时会下载并校验 Android 版 Mihomo。可在终端内管理订阅、切换节点和测速，也可用 `termihomo start` 启动本地 HTTP/SOCKS 代理，默认地址为 `127.0.0.1:7890`。终端界面至少需要 64 列、24 行。界面过窄时可缩小 Termux 字号或横屏使用。剪贴板按钮使用可选的 Termux:API。

Termux 版提供本地应用代理。全机 VPN、Android 系统代理和开机启动需要独立的 Android 集成。Android 可能回收锁屏后的 Termux 后台进程。配置默认保存在 Termux 私有目录 `~/.config/termihomo`。

Termihomo 在首次启动时自动下载当前版本配套的 Mihomo 并校验 SHA-256。更新 Termihomo 后，如果配套版本有变化，下次启动会自动下载对应内核。

TUI 和 CLI 支持简体中文与英文，默认英文，可在「工具 → 语言」切换并保存。用 `./bin/termihomo --lang zh-CN` 或 `./bin/termihomo --lang en` 指定本次运行的语言，也可设置 `TERMIHOMO_LANG` 作为常用语言。`--lang` 放在子命令前，例如 `./bin/termihomo --lang zh-CN status`。命令名与 `status --json` 的字段及状态值固定。Mihomo 原始日志和订阅中的节点名按原文显示。

TUI 支持点击页签、列表和底部操作按钮。滚轮浏览列表与日志历史，表单可点选光标位置，也可点击粘贴、保存和取消。仪表盘按 `a` 开关代理监听。关闭后仍可列出、选择和测速节点，重新开启会沿用已选节点。已有配置时，TUI 会在代理关闭的状态下加载内核。「工具 → 内核」或 CLI 的 `stop` 可以终止内核进程。「工具」提供内核管理、网络设置、DNS 查询和日志，「请求」可查看和关闭连接。

按 `Tab` 或 `1`–`5` 按侧栏顺序切页，工具用页内页签或 `[` / `]` 切换内容，代理网格用方向键或 `hjkl` 移动，其他列表用 `j` / `k` 移动，`gg` / `End` 跳到首尾，`Ctrl+U` / `Ctrl+D` 翻半页，`Ctrl+B` / `Ctrl+F` 翻整页。`/` 筛选，`Enter` 的作用取决于当前页，`Esc` 返回或清除筛选，日志级别用`v` 切换。`?` 查看快捷键，`q` 退出。

配置页按 `u` 批量更新订阅，`e` 编辑活动配置的名称或订阅链接；可用 `a` 导入链接或 `i` 导入本地 YAML。流量额度与到期时间取自订阅服务商，之前已导入的订阅可更新一次以读取额度。

节点页按 `f` 测试全部代理节点，`t` 测试焦点节点，`s` 在默认顺序和延迟排序间切换。

Mihomo 运行时，仪表盘的「出站模式」面板可用 `b` 切换规则、`m` 切换全局、`d` 切换直连。全局模式下进入代理页可在 GLOBAL 组选择出口。规则、全局和直连决定 Mihomo 如何转发流量。独立的 `u` 开关控制 TUN 是否接管系统路由，Linux 下需要 `/dev/net/tun`。首次启用时，Termihomo 通过 `pkexec` 请求系统授权，授权成功后重启 Mihomo 并启用 TUN。在 Windows 上，请通过管理员终端启动 Mihomo 后启用 TUN。在 macOS 上，使用具有管理员权限的 Mihomo（例如用 `sudo` 运行 Termihomo，root 使用独立配置目录）。TUN 保留系统现有的 DNS 配置。重装或升级内核后，新文件可能需要重新授权。TUN 关闭时，应用可通过总览显示的本地 HTTP/SOCKS 地址使用代理。模式和 TUN 修改对当前内核会话生效。

仪表盘的出站模式用 `●` 标记当前选项。按 `w` 开关局域网代理，开启后，同一私有 IPv4 子网内的设备可使用总览显示的内网地址和端口。此开关默认关闭，内核重启或重载配置后恢复关闭。

代理页按配置顺序展示策略组，默认使用组内成员的原顺序。可以点击顶部策略组，或按 `[` / `]` 切换。点击节点卡片移动焦点，按 `Enter` 将焦点节点应用到当前手动策略组，按 `e` 查看详情。

「请求」中的连接表显示进程、目标、协议、规则、出口、实时速率、累计流量和时长。点击顶部标签可多选或取消筛选，也可按 `Ctrl+P` / `Ctrl+T` 打开协议或规则菜单后按回车切换选项，按 `Esc` 关闭菜单。请求页只显示运行中的连接。按 `c` 分类，窄屏下用 `h` / `l` 横移。系统代理通过仪表盘的 `p` 手动开启。

「工具 → 常规」可设置代理端口、启动行为、切换节点时关闭连接、测速地址和超时。「工具 → DNS」展示实时查询记录，支持筛选、手动查询和清除缓存。选中设置后按回车编辑、按 `d` 恢复继承。内核设置保存后可按 `Ctrl+R` 应用并重启。

```sh
./bin/termihomo status --json
./bin/termihomo proxy list
./bin/termihomo proxy select '策略组' '节点'
./bin/termihomo logs -f
./bin/termihomo stop --yes
```

`status --json` 的 `proxy` 字段是当前 HTTP/SOCKS 代理地址。默认使用 `127.0.0.1:7890`。端口已占用时，启动会报错。可以在「工具 → 常规」指定其他端口。退出码 `0` 表示命令成功执行，`1` 表示操作失败，`2` 表示参数错误或非交互操作缺少确认。查询成功时请读取 JSON 的 `state` 字段判断 Mihomo 状态。

使用订阅时，可用 `./bin/termihomo profile add -` 从标准输入读取 URL，避免把订阅凭据放进命令行。随后运行 `./bin/termihomo profile update` 手动刷新。配置页可管理多份配置，接受完整的 Mihomo YAML。运行配置由 Termihomo 生成，Controller 和混合代理端口使用本机回环地址，导入配置中的额外监听器和 TUN 设置会被移除。`logs` 查看 Termihomo 生命周期记录，`logs -f` 还会接收 Mihomo 实时日志。

「自动运行」在打开界面时启动 Mihomo。「开机启动」分别通过 Linux systemd、macOS launchd 和 Windows 任务计划程序在开机时启动 Mihomo。macOS 会请求管理员授权，Windows 需要从管理员终端启用。系统代理和排除域名分别使用 GNOME、macOS 活动网络服务和 Windows 用户代理设置。

TUI 的状态刷新间隔和快捷键保存在 `settings.yaml`。默认路径如下，设置绝对路径的 `XDG_CONFIG_HOME` 后，配置使用其下的 `termihomo/settings.yaml`。

| 系统 | 默认路径 |
|---|---|
| Linux | `~/.config/termihomo/settings.yaml` |
| macOS | `~/Library/Application Support/termihomo/config/settings.yaml` |
| Windows | `%LOCALAPPDATA%/termihomo/config/settings.yaml` |

支持 `quit`、`help`、`refresh`、`search`、`sort` 和 `log-level`。例如：

```yaml
refresh-seconds: 5
keys:
  search: z
```

`go test ./...` 和 `go vet ./...` 检查 Go 代码。`python3 test/smoke.py ./bin/termihomo` 会在临时目录下载校验官方内核，并通过本机 HTTP 服务和终端验证代理、配置回滚及后台运行。测试需要联网和 `curl`。

- [技术架构](docs/ARCHITECTURE.md)
- [Mihomo 集成验证记录](docs/TECHNICAL_RESEARCH.md)
