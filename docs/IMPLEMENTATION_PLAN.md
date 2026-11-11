# Termihomo 分阶段实施计划（IMPLEMENTATION_PLAN）

> 目标：交给 Codex 按阶段实施  
> 开发语言：Go  
> 首发平台：Linux AMD64 / ARM64  
> 内核：Mihomo  
> 产品形态：TUI + CLI  
> 重要规则：每个阶段完成验收后再进入下一阶段，不以“代码已写”代替“功能已验证”。

## 1. 执行原则

1. 先调研和验证 Mihomo 当前稳定版本的下载资产、Controller API、配置校验和许可证，再写集成代码。
2. 优先完成 Linux 端到端闭环，不同时铺开所有平台。
3. 三层架构从第一天建立：Core Manager、Proxy Manager、Terminal Experience。
4. TUI 和 CLI 共用应用服务，不重复实现业务逻辑。
5. 首版使用本机 loopback HTTP Controller + WebSocket；不要未经验证就假设 Mihomo 支持 Unix Socket 或 Windows Named Pipe Controller。
6. Mihomo 作为独立进程运行；TUI 退出不停止内核。
7. 每一阶段必须有测试、验收记录和可运行产物。
8. 不为了架构“高级”引入不必要的微服务、数据库或复杂 IPC。

## 2. 开发前技术调研（Gate 0）

在实现前完成以下核查：

### 2.1 Mihomo 内核

- 确认官方仓库和当前维护状态。
- 确认目标稳定版本和发行资产命名。
- 确认 Linux AMD64、Linux ARM64 资产可用。
- 确认未来 macOS、Windows、Android 的资产情况。
- 确认下载校验信息及其可信来源。
- 确认内核许可证和分发义务。
- 确认配置校验方式。
- 确认 External Controller API 的认证方式。
- 确认 Traffic、Logs、Connections、Proxies、Groups、Rules 的实际 API 能力。
- 确认 WebSocket 事件格式、重连和错误行为。
- 核实 Controller 是否支持 Unix Socket 或其他本地 IPC，不允许猜测。

### 2.2 Go 依赖

- 检查 Bubble Tea 当前稳定版本及其终端兼容性。
- 检查 Lip Gloss 当前稳定版本。
- 检查 Cobra 当前稳定版本。
- 选择 WebSocket 库并核实维护状态和许可证。
- 选择 YAML 解析/校验依赖。
- 检查所有依赖许可证。
- 建立 Go 版本和 CI 构建矩阵。

### Gate 0 验收

输出 docs/TECHNICAL_RESEARCH.md，包含官方链接、版本、平台资产、API 端点、已知限制、许可证结论和未决问题。关键事实必须有来源，未核实事项明确标记。

## 3. 阶段一：项目骨架与平台基础（Phase 1）

### 任务

- 初始化 Go Module。
- 建立目录结构和依赖注入入口。
- 实现版本信息和 termihomo version。
- 实现配置目录、数据目录和日志目录解析。
- 实现结构化日志和敏感信息脱敏。
- 实现 OS / CPU 架构检测。
- 实现单实例锁的基础设计。
- 实现 termihomo --help。
- 建立 Linux AMD64 / ARM64 CI 构建。
- 建立 go test ./... 和 go vet ./...。

### 验收

- 两种 Linux 架构均可构建。
- 首次运行能创建数据目录。
- 不同用户数据不会混用。
- 日志不输出测试 Secret。
- CLI 帮助和版本命令正常。
- 基础测试全部通过。

## 4. 阶段二：Core Manager（Phase 2）

### 任务

#### 4.1 Mihomo 安装器

- 获取可用版本信息。
- 选择平台对应资产。
- 下载到临时目录。
- 校验摘要或官方验证信息。
- 解压并检查文件。
- 安装到版本独立目录。
- 记录当前版本。
- 支持失败清理，不覆盖现有可用版本。

#### 4.2 进程管理

- 启动 Mihomo 子进程。
- 记录 PID 和实例标识。
- 检测进程存活。
- 检测 Controller 健康状态。
- 停止和重启。
- 处理启动超时和启动失败。
- TUI 退出时保留进程。
- 处理孤儿进程和过期 PID 文件。
- 不允许根据单纯 PID 数字误杀进程。

#### 4.3 Controller 配置

- 生成随机 API Secret。
- Controller 仅绑定 loopback。
- 选择可用端口并避免端口竞争。
- 将 Controller 信息安全传递给应用服务。
- 不把 Secret 输出到日志。
- 处理进程已启动但 API 不可用的 Degraded 状态。

### 验收

- 全新 Linux 环境可自动下载和安装 Mihomo。
- 能启动、停止、重启。
- termihomo status 能区分 Stopped、Running、Degraded、Failed。
- 退出 CLI/TUI 前端不会杀掉后台内核。
- Controller 不监听公网地址。
- 错误时保留旧版本和原有配置。

## 5. 阶段三：Proxy Manager 最小闭环（Phase 3）

### 任务

#### 5.1 Profile

- 添加订阅 URL。
- 导入本地 YAML。
- 保存 Profile 元数据。
- 下载订阅到临时文件。
- YAML 语法检查。
- Mihomo 配置校验。
- 校验通过后备份并替换。
- 失败时保留旧配置。
- 加载指定 Profile。

#### 5.2 Proxies / Groups

- 获取代理列表。
- 获取策略组。
- 获取组内选项和当前选项。
- 支持可选择策略组切换。
- 获取节点延迟。
- 发起单节点测速。
- 处理 API 错误和节点不可用状态。

#### 5.3 Traffic / Logs

- 连接 Traffic WebSocket。
- 连接 Logs WebSocket。
- 正确处理断线和重连。
- 解析事件并更新内存状态。
- 断线时显示数据已过期，不伪造实时状态。

### 验收

- 能添加并加载有效订阅。
- 能显示节点和策略组。
- 能切换一个可选择策略组。
- 能发起测速。
- 能看到持续更新的流量。
- 能看到实时日志。
- 更新无效配置不会破坏当前可用配置。

## 6. 阶段四：Terminal Experience（Phase 4）

### 任务

#### 6.1 TUI 基础

- 实现主应用模型。
- 实现 Dashboard。
- 实现页面导航。
- 实现统一状态栏。
- 实现键盘快捷键帮助。
- 实现终端尺寸变化处理。
- 实现退出时恢复终端状态。
- 实现连接中、加载中、空数据、错误、过期数据等状态。

#### 6.2 页面

按顺序实现：

1. Dashboard。
2. Proxies。
3. Groups。
4. Profiles。
5. Traffic。
6. Logs。
7. Core。

Rules、Connections、DNS、Config 编辑器可在后续阶段完善，不应为了页面数量牺牲核心闭环质量。

#### 6.3 CLI

实现：

```text
termihomo
termihomo start
termihomo stop
termihomo restart
termihomo status
termihomo profile list
termihomo profile add <url>
termihomo profile update [name]
termihomo proxy list
termihomo proxy test
termihomo logs
termihomo logs -f
termihomo config check
termihomo config backup
termihomo version
```

CLI 与 TUI 必须使用同一组 Application Services。

### 验收

- Dashboard 数据正确。
- 节点切换在 TUI 中完成。
- TUI 退出后 Mihomo 继续运行。
- 重新打开 TUI 可恢复连接。
- CLI 命令能独立工作。
- JSON 输出可供脚本解析。
- 小终端尺寸下没有崩溃或无法退出的问题。

## 7. 阶段五：配置、连接与规则完善（Phase 5）

### 任务

- Connections 列表、过滤和详情。
- 关闭单个连接。
- Rules 列表和过滤。
- Rule Providers 状态和更新。
- DNS 配置查看。
- Config YAML 查看。
- YAML 编辑器或外部编辑器集成方案。
- 配置 Diff。
- 配置备份。
- 安全校验和重载。
- 重载失败恢复旧配置。
- 自动更新订阅周期。
- 订阅更新失败通知。

### 验收

- 连接信息与内核 API 一致。
- 关闭连接不会误关闭其他连接。
- 规则功能只展示内核实际提供的信息。
- 配置保存失败不会破坏当前运行配置。
- 重载失败能恢复旧配置。
- 更新订阅失败保留旧配置。

## 8. 阶段六：平台扩展（Phase 6）

按以下顺序扩展：

### 6.1 macOS

- AMD64 和 ARM64 构建。
- 路径和权限适配。
- 进程管理。
- 后台运行。
- launchd 集成作为可选功能。
- 终端能力测试。

### 6.2 Windows

- AMD64 优先。
- 路径、进程和信号适配。
- PowerShell / Windows Terminal 测试。
- 后台进程生命周期。
- Windows Service 作为可选增强。
- 不要求普通代理模式必须有管理员权限。
- Named Pipe 仅在有明确服务端支持时使用。

### 6.3 Android Termux

- Android ARM64 Go 构建。
- Mihomo Android 对应构建验证。
- Bionic 兼容性测试。
- Termux 路径和权限适配。
- 竖屏和窄屏 TUI。
- 后台运行限制和电池优化说明。
- TUN 能力单独评估，不列为普通 Termux 首版承诺。

### 6.4 Docker

- 容器内运行 Termihomo 和 Mihomo。
- 支持 -it 交互式 TUI。
- 使用 Volume 持久化配置。
- 正确处理容器停止信号。
- 明确端口映射。
- TUN 需要额外设备和权限说明。
- 提供最小 Dockerfile / Compose 示例。

### 验收

每个平台分别提供构建、启动、订阅加载、节点切换、流量显示和退出行为测试结果。不得只以交叉编译成功作为平台支持完成的依据。

## 9. 阶段七：发布与 V1.0（Phase 7）

- GoReleaser 多平台发布。
- 发布校验和。
- 自动生成 Release Notes。
- 安装脚本。
- 升级和回滚说明。
- Docker 镜像。
- 中英文 README。
- 中英文用户文档。
- 常见故障排查。
- 许可证和第三方声明。
- 安全默认值检查。
- 完整回归测试。
- 发布前端到端验收。

## 10. 测试策略

### 单元测试

- 平台识别。
- 路径解析。
- 下载校验。
- 版本选择。
- 状态机。
- 配置备份和原子替换。
- Secret 脱敏。
- CLI 参数和退出码。

### 集成测试

- Mihomo 进程启动。
- Controller API 认证。
- 配置校验和加载。
- 节点和策略组查询。
- 节点切换。
- Traffic / Logs WebSocket。
- 进程异常退出。
- TUI 退出后内核继续运行。

### 端到端测试

- 全新 Linux 安装。
- 添加订阅。
- 启动内核。
- 实际代理请求验证。
- 切换节点。
- 观察流量。
- 退出并重新进入。
- 配置错误和恢复。
- 后台进程异常处理。

### 平台测试

每个平台记录：

- OS 版本。
- CPU 架构。
- Go 版本。
- Mihomo 版本。
- 终端类型。
- 是否有管理员/root 权限。
- 测试通过的功能。
- 不支持或受限的功能。

## 11. Codex 执行规则

Codex 实施时必须遵守：

1. 先阅读 PRODUCT_SPEC.md 和 ARCHITECTURE.md。
2. 先完成 Gate 0 技术调研，不允许猜测 Mihomo API。
3. 每次只实施一个 Phase。
4. 实施前列出涉及文件和设计方案。
5. 不擅自改变三层架构。
6. 不自行重写 Mihomo 已有能力。
7. 不引入不必要的数据库、守护服务或微服务。
8. 不把 TUI 退出等同于停止 Mihomo。
9. 每个功能必须有测试。
10. 每个 Phase 完成后输出：变更摘要、测试结果、未完成事项、已知限制和验收结论。
11. 测试未通过时明确标记，不得声称完成。
12. 不在没有确认的情况下进行破坏性操作或覆盖用户已有配置。

## 12. 首个 Codex 任务

建议首先只执行 Gate 0 和 Phase 1 的调研/骨架工作，不要直接实现全部功能。

完成后提交以下结果：

- docs/TECHNICAL_RESEARCH.md
- 初始 Go 项目结构
- 平台检测和数据目录模块
- CLI version/help
- Linux AMD64 / ARM64 CI
- 单元测试和构建结果
- 未决技术问题清单

通过验收后再进入 Mihomo 安装和进程管理阶段。
