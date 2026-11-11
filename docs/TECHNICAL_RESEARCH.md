# Termihomo Gate 0 技术调研

核查日期：2026-11-11（Asia/Shanghai）。**Gate 0 通过，可进入 Phase 1。** 已下载校验 Linux 双架构发布物，并在 AMD64 上用官方内核实测配置校验、认证、节点切换、真实代理请求、流量与日志。ARM64 原生运行、终端兼容性和 Termihomo 自身的恢复流程留给对应实施阶段验收。

## 发行资产与完整性

[官方稳定版 API](https://api.github.com/repos/MetaCubeX/mihomo/releases/latest) 本次返回 [v1.19.34](https://github.com/MetaCubeX/mihomo/releases/tag/v1.19.34)，`draft=false`、`prerelease=false`，发布于 2026-11-11 15:01（北京时间）。[官方构建命名说明](https://github.com/MetaCubeX/mihomo/wiki/FAQ) 区分 AMD64 微架构级别；首版选择 v1 资产，避免要求 v2/v3 指令集。

| Linux 目标 | 资产名 | 官方 API 中 gzip 的 SHA-256 |
|---|---|---|
| AMD64 | `mihomo-linux-amd64-v1-v1.19.34.gz` | `0c07340a01408d033dda4088ea2a4c1e4d82aeed7b2f84f7c046e93b6ff3290c` |
| ARM64 | `mihomo-linux-arm64-v1.19.34.gz` | `daaf78a6240a282cdfd75bcbf98fe321f6bd52765cc29e79f344191198f0b50f` |

两份 gzip 均从官方 Release 下载，`sha256sum -c` 校验通过，解压后分别确认为静态链接的 x86-64 / AArch64 ELF。ARM64 产物未在原生环境运行。该版本另有 `mihomo-darwin-{amd64-v1,arm64}-v1.19.34.gz`、`mihomo-windows-{amd64-v1,arm64}-v1.19.34.zip` 和 `mihomo-android-arm64-v8-v1.19.34.gz`；其存在由发布元数据确认，未运行。Termux 须独立验证 Android 构建。

安装器应在官方仓库的指定 tag 下精确匹配资产名，使用 GitHub Release API 的 [asset digest](https://docs.github.com/en/rest/releases/assets?apiVersion=2022-11-28#get-a-release-asset) 校验压缩包，再解压并安装；缺少摘要、算法不是 SHA-256 或校验失败时停止且保留现有安装。集成测试固定 tag 和摘要，避免测试目标随 latest 变动。此摘要可验证下载内容与官方发布元数据一致；不等同于独立签名或构建证明，本次未验证后两者。

## Linux AMD64 实测

使用临时工作目录、随机 Secret 和回环监听。配置含一个 `select` 组（可选 `REJECT` 和 `DIRECT`），规则为 `MATCH,Gate`；通过 Mihomo mixed port 向本机 HTTP 测试服务发出真实请求。测试不依赖公开代理节点。内核通过 `mihomo -d <workdir> -f <config.yaml>` 启动。

| 能力 | 实测结果 |
|---|---|
| 配置校验 | `mihomo -d <workdir> -t -f <config.yaml>`；有效配置退出 0，语法错误退出 1。 |
| 版本与认证 | `GET /version` 带 `Authorization: Bearer <secret>` 返回 200 和 `{"meta":true,"version":"v1.19.34"}`，无认证返回 401。 |
| 节点和策略组 | `GET /proxies`、`GET /group/Gate` 返回节点和组状态。向 `PUT /proxies/Gate` 提交 `{"name":"DIRECT"}` 返回 204；组当前选项变更后，经 mixed port 访问测试服务成功。对非组 `DIRECT` 执行选择返回 400。 |
| 延迟 | `GET /proxies/DIRECT/delay?url=<encoded>&timeout=3000` 返回以毫秒计的 `delay`。 |
| 流量 | Bearer 认证升级 `/traffic` 为 WebSocket，收到 `up`、`down`、`upTotal`、`downTotal`；请求后累计流量增长。断开并重新连接后可再次收到采样。 |
| 日志 | Bearer 认证升级 `/logs?level=info` 为 WebSocket，代理请求产生含 `type`、`payload` 的 JSON 消息。 |
| 连接、规则 | `GET /connections` 返回连接数组及累计计数；`GET /rules` 返回当前规则及部分统计。没有实测活动连接关闭和 rule provider 更新。 |
| 配置重载 | `GET /configs` 返回运行时信息。向 `PUT /configs` 提交无效 YAML 的 `payload` 返回 400，运行中的组选择保持；提交 `{}` 从默认配置路径重载，返回 204。有效重载可能改变运行时选项：测试中未持久化选项，重载后恢复默认 `REJECT`。 |
| Unix Socket | 配置 `external-controller-unix: controller.sock` 后，通过 Unix Socket 不带 Bearer 调用 `GET /version` 成功。 |

来源：[官方 API 说明](https://wiki.metacubex.one/en/api/)、固定版本的 [CLI 参数](https://github.com/MetaCubeX/mihomo/blob/v1.19.34/main.go)、[认证与流端点](https://github.com/MetaCubeX/mihomo/blob/v1.19.34/hub/route/server.go)、[组选择与测速](https://github.com/MetaCubeX/mihomo/blob/v1.19.34/hub/route/proxies.go)、[连接](https://github.com/MetaCubeX/mihomo/blob/v1.19.34/hub/route/connections.go)、[规则](https://github.com/MetaCubeX/mihomo/blob/v1.19.34/hub/route/rules.go)、[配置重载](https://github.com/MetaCubeX/mihomo/blob/v1.19.34/hub/route/configs.go)。

## 集成约束

- **状态分开探测。** 实测出现过 `/version` 已返回 200，但规则和代理组尚不可用的窗口。[固定版本启动顺序](https://github.com/MetaCubeX/mihomo/blob/v1.19.34/hub/hub.go) 先建立 Controller，再加载代理规则与监听配置。保留进程存活、Controller 可访问和配置可用的区分；启动验收验证预期配置或代理数据路径。
- **保持 Controller 配置归 Termihomo。** 首版只启用回环 HTTP + WebSocket，均用 Bearer；Secret 放在权限受限的运行配置，避免出现在命令行、URL 或日志。组选择按受支持类型暴露，服务端仍可能拒绝候选项。WebSocket 断开后由客户端退避重连，期间标注数据已过期；目前实测连接恢复，未确认日志重放。[Controller 配置文档](https://wiki.metacubex.one/en/config/general/) 和 [固定版本源码](https://github.com/MetaCubeX/mihomo/blob/v1.19.34/hub/route/server.go) 均说明 Unix Socket、Windows Named Pipe 的原生入口；源码中两者均未启用 Bearer，且 Unix Socket 设为 0666。Windows Pipe 未实测，首版两者均不启用。
- **受管配置和订阅原文分离。** 生成受管运行文件时明确覆盖 Controller、安全相关监听与 Secret，避免导入文件重新打开额外 Controller。用相同内核、工作目录和权限校验候选配置。优先以 `PUT /configs` 和 `{}` 重载固定受管路径；显式 `path` 受绝对路径和安全目录限制。204 表示重载处理结束，不能单独证明代理请求已恢复；失败恢复要验证旧文件和运行状态，并明确报告恢复失败。[配置处理](https://github.com/MetaCubeX/mihomo/blob/v1.19.34/hub/route/configs.go)；[应用配置流程](https://github.com/MetaCubeX/mihomo/blob/v1.19.34/hub/executor/executor.go)。

## 工具链与候选依赖

以 Go 1.26 为基线，CI 用该分支最新修补版本；[Go 官方发布记录](https://go.dev/doc/devel/release#go1.26) 在调研日列出 1.26.11，本机已下载并验证 `GOTOOLCHAIN=go1.26.11 go version`。Phase 1 优先使用标准库，未使用的候选库不加入模块。

| 用途 | 调研时稳定候选 | go.mod 最低版本 | 上游许可 |
|---|---|---|---|
| TUI | [Bubble Tea v2.1.1](https://github.com/charmbracelet/bubbletea/tree/v2.1.1) | 1.26.0 | MIT |
| 样式 | [Lip Gloss v2.1.0](https://github.com/charmbracelet/lipgloss/tree/v2.1.0) | 1.26.7 | MIT |
| CLI | [Cobra v1.10.2](https://github.com/spf13/cobra/tree/v1.10.2) | 1.15 | Apache-2.0 |
| WebSocket | [Coder WebSocket v1.8.15](https://github.com/coder/websocket/tree/v1.8.15) | 1.23 | ISC |
| YAML | [Go YAML v3.0.6](https://github.com/yaml/go-yaml/tree/v3.0.6) | 1.16 | 按文件区分 MIT、Apache-2.0 |

版本及最低 Go 版本来自 Go 模块元数据，许可由下载模块的 LICENSE / NOTICE 复核。Phase 4 加入 UI 依赖时，实测常见终端、SSH/PTY、窄屏、调整窗口大小与退出后恢复终端状态。构建矩阵使用 [GitHub 托管运行器](https://docs.github.com/en/actions/reference/runners/github-hosted-runners) 的 `ubuntu-24.04`（AMD64）和 `ubuntu-24.04-arm`（ARM64），分别原生运行 build/test/vet；可用的官方 action 版本为 [checkout v7.0.1](https://github.com/actions/checkout/releases/tag/v7.0.1) 和 [setup-go v7.0.0](https://github.com/actions/setup-go/releases/tag/v7.0.0)。本地交叉编译不替代 ARM64 原生运行验证。

## 许可与未决验收

- **核对具体 tag。** [v1.19.34 的 LICENSE](https://github.com/MetaCubeX/mihomo/blob/v1.19.34/LICENSE) 和 [该 tag 的 README](https://github.com/MetaCubeX/mihomo/blob/v1.19.34/README.md) 标明 GPL-3.0；[默认分支 LICENSE](https://github.com/MetaCubeX/mihomo/blob/main/LICENSE) 在核查时不同，不能用来判断该发布物的条款。V0.1 推荐从官方 Release 下载独立进程，Termihomo 发布包不捆绑、不链接内核。如将来通过镜像、安装包或镜像站分发内核二进制，发布前应按目标 tag 检查许可声明和对应源码提供方式，见 [GPL v3 第 4–6 节](https://www.gnu.org/licenses/gpl-3.0.html)。
- Termihomo 自身许可证和实际引入的传递依赖清单在发布前确定。当前候选库尚未成为发布依赖。
- Phase 2/3 还需验证 ARM64 原生运行、非 root 管理和 PID 身份校验、端口冲突、后台跨会话、订阅更新失败后的回滚、断线期间的过期提示。macOS、Windows、Termux、Docker、TUN 留到各平台验收。此调研未将这些项目视为已经通过。
