# Termihomo

[English](README.en.md)

Termihomo 是 Mihomo 的终端管理工具。它提供 TUI 和可脚本化 CLI。退出 TUI 后，托管的 Mihomo 继续在后台代理。

首版面向 Linux AMD64 / ARM64，需要 Linux 5.3 或更新版本来安全识别和停止托管进程。使用 Go 1.26 构建。

```sh
go build -o termihomo ./cmd/termihomo
./termihomo install
./termihomo profile import /path/to/config.yaml
./termihomo start
./termihomo
```

TUI 和 CLI 支持简体中文与英文，默认跟随终端的语言设置。用 `./termihomo --lang zh-CN` 或 `./termihomo --lang en` 指定本次运行的语言，也可设置 `TERMIHOMO_LANG` 作为常用语言。`--lang` 放在子命令前，例如 `./termihomo --lang zh-CN status`。命令名与 `status --json` 的字段及状态值固定。Mihomo 原始日志和订阅中的节点名按原文显示。

TUI 内按 `Tab` 或 `1`–`7` 切换页面，按 `?` 查看快捷键，按 `q` 退出。也可以在 Core 页面安装、启动、停止或重启 Mihomo。Proxy 页支持搜索、排序和测速，Groups 页支持查看组内选项和切换 Selector。Traffic 和 Logs 页显示实时数据。

```sh
./termihomo status --json
./termihomo proxy list
./termihomo proxy select '策略组' '节点'
./termihomo logs -f
./termihomo stop --yes
```

`status --json` 的 `proxy` 字段是当前 HTTP/SOCKS 代理地址。默认尝试 `127.0.0.1:7890`，端口已占用时选择其他可用的回环端口。退出码 `0` 表示命令成功执行，`1` 表示操作失败，`2` 表示参数错误或非交互操作缺少确认。查询成功时请读取 JSON 的 `state` 字段判断 Mihomo 状态。

使用订阅时，可用 `./termihomo profile add -` 从标准输入读取 URL，避免把订阅凭据放进命令行。随后运行 `./termihomo profile update` 手动刷新。首版管理一个 Profile，接受完整的 Mihomo YAML。运行配置由 Termihomo 生成，Controller 和混合代理端口使用本机回环地址，导入配置中的额外监听器和 TUN 设置会被移除。`logs` 查看 Termihomo 生命周期记录，`logs -f` 还会接收 Mihomo 实时日志。

可在 `${XDG_CONFIG_HOME:-$HOME/.config}/termihomo/settings.yaml` 调整 TUI 的状态刷新间隔和快捷键。支持 `quit`、`help`、`refresh`、`search`、`sort`、`test` 和 `log-level`。例如：

```yaml
refresh-seconds: 5
keys:
  search: f
  test: d
```

`go test ./...` 和 `go vet ./...` 检查 Go 代码。`python3 test/smoke.py ./termihomo` 会在临时目录下载校验官方内核，并通过本机 HTTP 服务和终端验证代理、配置回滚及后台运行。测试需要联网和 `curl`。

- [产品规格](docs/PRODUCT_SPEC.md)
- [技术架构](docs/ARCHITECTURE.md)
- [实施计划](docs/IMPLEMENTATION_PLAN.md)
- [Mihomo 调研与实测记录](docs/TECHNICAL_RESEARCH.md)
