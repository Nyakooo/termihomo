# Termihomo 技术架构

Termihomo 支持 Linux、macOS 和 Windows 的 AMD64 和 ARM64。TUI 和 CLI 使用同一个 `core.Manager` 管理独立的 Mihomo 进程。流量由 Mihomo 转发，退出 TUI 后内核仍可运行。

## 代码入口

| 目录 | 职责 |
|---|---|
| `cmd/termihomo`、`internal/cli` | 命令行入口、参数、确认和输出 |
| `internal/tui` | Bubble Tea 界面、快捷键、鼠标操作和运行状态 |
| `internal/core` | 内核下载及校验、进程管理、配置库、控制接口和系统网络设置 |
| `internal/platform` | 平台默认目录、XDG 覆盖路径、权限检查和实例锁 |
| `internal/i18n` | 英文与简体中文界面文本、诊断和语言偏好 |

## 代理页

- `internal/core/proxies.go` 读取 Mihomo 的 `/proxies`，用受管 YAML 的 `proxy-groups` 序列恢复策略组顺序。组内顺序来自 Mihomo 返回的 `all` 数组。
- `internal/tui/proxy_grid.go` 生成顶部策略组和节点网格。规则模式显示可见组，全局模式显示全部组，直连模式隐藏策略组。打开页面时默认使用第一个可见组。
- 键盘和鼠标活动经 `internal/tui/input.go` 分发。点击节点时，只有 Selector 组可手动切换。内核写入后读回当前选项，确认是否生效。
- 单节点测速调用 Mihomo 的节点延迟接口。全量测速复用同一接口，最多同时发出八个请求，逐个更新节点结果。延迟排序由 TUI 控制。

## 配置与进程

- 订阅与本地 YAML 保存在配置库。核心运行使用单独生成的配置，控制接口绑定回环地址并通过 Bearer Secret 认证。导入配置中的额外监听器和 TUN 设置在生成时移除。
- 候选配置经 Mihomo 校验后再替换运行配置。更新失败时使用先前配置恢复，实例锁串行化会修改进程或运行配置的操作。
- 进程管理会核对进程身份。Linux 用 pidfd，Windows 用进程句柄和创建时间，macOS 用进程创建时间和 kqueue 退出通知。启动检测同时检查控制接口和代理监听器。流量与日志使用 WebSocket，连接断开后重试。
- 出站模式、桌面系统代理、TUN 和局域网代理有独立控制入口。应用从内核或系统读取当前状态。

## 验证

运行 `go test ./...` 和 `go vet ./...`。构建后可用 `python3 test/smoke.py ./bin/termihomo` 在隔离目录验证真实代理请求、配置回滚和后台进程。固定内核版本的原始实测见 [Mihomo 集成验证记录](TECHNICAL_RESEARCH.md)。
