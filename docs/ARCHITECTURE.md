# Termihomo 技术架构设计（ARCHITECTURE）

> 状态：建议架构基线  
> 语言：Go  
> 核心：Mihomo 独立进程  
> 形态：TUI + CLI  
> 原则：三层解耦、跨平台适配、业务逻辑复用、控制平面与数据平面分离

## 1. 架构概览

Termihomo 不实现代理数据转发。Mihomo 负责网络代理数据平面；Termihomo 负责控制平面、管理、监控和交互。

```text
┌─────────────────────────────────────────────────────────┐
│                 Terminal Experience                     │
│       TUI (Bubble Tea)              CLI                  │
└──────────────────────────┬──────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────┐
│                    Application Services                 │
│  Core Service │ Profile Service │ Proxy Service         │
│  Group Service│ Rules Service   │ Connection Service    │
│  Traffic      │ Logs            │ Config Service        │
└──────────────────────────┬──────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────┐
│                 Core Manager / Core Client              │
│  Install │ Version │ Process │ Health │ Lifecycle        │
│  Controller Client │ Event Streams │ State Reconciliation│
└──────────────────────────┬──────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────┐
│                   Transport Adapters                    │
│       Local HTTP/WebSocket (initial)                     │
│       Optional Unix Socket / Platform IPC (validated)    │
│       Remote HTTPS/WSS (future)                           │
└──────────────────────────┬──────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────┐
│                  Mihomo Core Process                     │
│      Proxy protocols / DNS / Rules / TUN / Routing       │
└─────────────────────────────────────────────────────────┘
```

## 2. 三层职责边界

### 2.1 Core Manager

只负责内核本身：

- 平台检测。
- 发行版本查询和下载。
- 下载校验。
- 安装目录和版本目录管理。
- 进程启动、停止、重启。
- 进程状态和 API 健康状态。
- 后台进程生命周期。
- 内核日志位置和启动参数。
- 内核升级和回滚。

Core Manager 不负责节点列表 UI、订阅交互或流量图表。

### 2.2 Proxy Manager

负责 Mihomo 提供的代理控制能力：

- Profile 和配置文件管理。
- Proxies 和 Groups。
- Rules 和 Rule Providers。
- Connections。
- Traffic。
- Logs。
- DNS 相关配置。
- YAML 配置编辑、校验、备份和重载。

Proxy Manager 不直接启动操作系统服务，也不负责 TUI 渲染。

### 2.3 Terminal Experience

只消费应用服务接口：

- Dashboard 和各管理页面。
- 键盘事件、鼠标事件和焦点管理。
- 表格、列表、图表、表单、弹窗。
- 页面刷新和实时数据呈现。
- CLI 命令解析和输出格式。

TUI 和 CLI 不应直接调用 Mihomo HTTP API，也不应直接操作配置文件；统一经过 Application Services。

## 3. Go 项目分层建议

建议采用清晰、不过度抽象的目录结构。以下是初始建议，实施时可按实际规模调整：

```text
termihomo/
├── cmd/
│   └── termihomo/
│       └── main.go
├── internal/
│   ├── app/
│   │   ├── bootstrap/
│   │   └── services/
│   ├── core/
│   │   ├── manager/
│   │   ├── installer/
│   │   ├── process/
│   │   └── health/
│   ├── proxy/
│   │   ├── profiles/
│   │   ├── proxies/
│   │   ├── groups/
│   │   ├── rules/
│   │   ├── connections/
│   │   ├── traffic/
│   │   ├── logs/
│   │   └── config/
│   ├── controller/
│   │   ├── client/
│   │   ├── transport/
│   │   └── events/
│   ├── platform/
│   │   ├── paths/
│   │   ├── process/
│   │   ├── service/
│   │   ├── permissions/
│   │   └── terminal/
│   ├── tui/
│   │   ├── app/
│   │   ├── pages/
│   │   ├── components/
│   │   └── styles/
│   ├── cli/
│   ├── storage/
│   ├── security/
│   └── version/
├── configs/
├── docs/
├── scripts/
├── test/
├── go.mod
└── README.md
```

### 分层规则

- `tui` 只调用应用服务，不直接访问 Controller。
- `cli` 只调用应用服务，不复制业务逻辑。
- `proxy` 通过 Controller Client 访问 Mihomo。
- `core/manager` 管理 Mihomo 二进制和进程。
- `platform` 隔离 OS 特有代码。
- `storage` 管理 Termihomo 自己的数据，不与 Mihomo 运行配置混为一谈。
- `security` 统一处理 Secret、脱敏和权限。
- 尽量避免为了抽象而创建大量接口；只有在跨平台、可测试或需要替换实现时才引入接口。

## 4. Mihomo 通信设计

### 4.1 控制平面与数据平面

- 代理数据流量直接由 Mihomo 处理。
- Termihomo 不代理、不转发、不包装用户的实际网络流量。
- Termihomo 只发送控制请求、接收状态数据和事件。
- TUI 的刷新频率不能影响 Mihomo 的代理转发路径。

### 4.2 首版通信建议

首版采用 Mihomo External Controller 的本机 HTTP API 和 WebSocket：

- Controller 只绑定 `127.0.0.1` 或等效 loopback 地址。
- 使用随机生成的 API Secret。
- API Secret 不写入日志，不展示在普通诊断输出中。
- WebSocket 用于流量、日志等持续事件。
- 普通 REST 请求用于节点列表、选择、配置、状态等操作。
- Connections 根据 API 能力采用合理刷新间隔，不进行高频无意义轮询。

这样可以先保证跨平台和实现简单，不需要首版就分别实现多种 IPC。

### 4.3 本地 IPC 的后续评估

Unix Domain Socket、Windows Named Pipe 等方式可以作为后续优化，但必须先验证目标 Mihomo 版本的 External Controller 是否原生支持对应传输方式。

不能仅因为 Go 标准库支持 Unix Socket 或 Named Pipe，就推断 Mihomo Controller 也支持这些协议。

若 Mihomo 不支持某种本地 IPC，Termihomo 不应自行假设可以直接连接；可继续使用受 loopback 限制并带 Secret 的 HTTP Controller。

### 4.4 远程管理

远程管理属于后续版本：

- 优先通过 SSH 隧道或经过认证的 HTTPS/WSS。
- 不直接暴露无 TLS 的 Controller 到公网。
- 远程连接与本机连接使用统一的 Controller Client 接口。
- 必须处理连接断开、超时、重连和权限错误。
- 远程多实例管理不进入 V0.1。

## 5. Core Manager 设计

### 5.1 安装器

建议职责：

1. 读取平台和架构。
2. 获取可用 Mihomo 发行版本。
3. 选择与平台匹配的发行资产。
4. 下载到临时目录。
5. 校验 SHA256 或发行源提供的验证信息。
6. 解压并检查可执行文件。
7. 安装到版本独立目录。
8. 原子更新当前版本指向。
9. 保留上一个可用版本以便回滚。

必须明确区分 Termihomo 自身版本与 Mihomo 内核版本。

### 5.2 进程管理

进程状态至少区分：

- NotInstalled：未安装。
- Stopped：已安装但未运行。
- Starting：启动中。
- Running：进程存活且 Controller 健康。
- Degraded：进程存活但 Controller 不可用或不健康。
- Stopping：停止中。
- Failed：启动失败或异常退出。
- External：检测到已有内核，但并非 Termihomo 创建或托管。

进程存活与 API 健康必须分开探测。

TUI 退出时不杀掉后台 Mihomo。CLI 的 `stop` 才执行停止操作。进程归属和 PID 文件必须防止 PID 重用导致误杀。

### 5.3 单实例与锁

- 防止同一个 Termihomo 管理进程被重复启动。
- 锁文件需记录进程信息和实例标识。
- 清理过期锁时必须验证进程身份。
- 连接已有内核与启动自管内核是不同的管理模式。
- V0.1 只管理一个本机实例。

## 6. Profile 与配置安全流程

订阅更新不得直接覆盖当前可用配置。

建议流程：

1. 下载到临时文件。
2. 限制下载大小并设置超时。
3. 检查响应状态和内容。
4. 执行 YAML 语法检查。
5. 使用匹配版本的 Mihomo 配置校验能力验证配置。
6. 验证通过后备份旧文件。
7. 原子替换配置文件。
8. 请求 Mihomo 安全重载。
9. 重载失败时恢复旧文件并报告错误。

订阅 URL 可能包含用户名、密码或 Token，应作为 Secret 处理。

### 配置编辑

- 编辑前读取当前配置。
- 保存前显示 Diff。
- 保存前备份。
- 写入临时文件并校验。
- 原子替换。
- 重载后检查健康状态。
- 失败时恢复旧配置。
- 不允许编辑器直接破坏正在使用的唯一配置文件。

## 7. 实时数据与 TUI 状态

建议建立内存中的统一状态模型：

- CoreState。
- ProfileState。
- ProxyState。
- GroupState。
- TrafficState。
- ConnectionState。
- LogState。
- UIState。

数据更新与 UI 渲染分离：

1. Controller Client 接收事件或请求数据。
2. 应用服务更新内存状态。
3. TUI 订阅状态变化并触发渲染。
4. 终端刷新不应直接触发重复网络请求。

建议：

- Traffic：优先使用 Mihomo WebSocket 事件。
- Logs：优先使用 WebSocket。
- Connections：按需刷新，支持手动刷新和适度定时刷新。
- Proxies / Groups：页面进入、操作完成或用户刷新时更新。
- Rules：按需加载，避免一次性读取过大配置。
- Dashboard：使用已有状态聚合，不重复请求各 API。

刷新周期必须可配置，并在终端失焦、页面不可见或窄屏模式下适当降低频率。

## 8. TUI 与 CLI 技术选择

建议候选：

- TUI：Bubble Tea。
- 样式：Lip Gloss。
- CLI：Cobra。
- 配置：YAML。
- HTTP：Go 标准库 `net/http`。
- WebSocket：选择维护活跃、许可证兼容的 Go 库。
- 日志：结构化日志，统一脱敏。
- 测试：Go 标准测试、httptest、平台构建检查。

依赖版本应在实现前检查当前稳定版本、兼容性和许可证，不在架构文档中锁死未经验证的版本号。

## 9. 平台适配

平台相关能力通过小型接口隔离：

- ProcessLauncher。
- ProcessInspector。
- PathResolver。
- PermissionChecker。
- ServiceManager。
- TerminalCapabilities。
- FileLock。

Linux、macOS、Windows、Android Termux 和 Docker 的差异放在平台实现中。

不要把 Docker 当作一个独立操作系统：Docker 容器内通常仍是 Linux 用户空间，但其权限、设备、网络命名空间和持久化方式有额外限制。

Termux 需要单独验证 Android/Bionic 的 Go 和 Mihomo 构建，不得直接假设普通 Linux ARM64 产物可用。

TUN 能力按平台实现，不能将其作为所有平台首版必须具备的共同能力。

## 10. 数据目录建议

目录路径由平台路径适配器决定，不在业务代码中硬编码。

建议逻辑上分离：

```text
Termihomo data/
├── app/
│   ├── settings.yaml
│   └── state.json
├── cores/
│   ├── versions/
│   └── current
├── profiles/
│   ├── profile-a.yaml
│   └── profile-b.yaml
├── backups/
├── logs/
└── secrets/
```

Mihomo 的运行配置和 Termihomo 的应用设置必须分开管理。Secret 文件采用最小权限。敏感订阅 URL 不应出现在文件名或日志中。

## 11. 可观测性与错误处理

统一错误分类：

- DownloadError。
- IntegrityError。
- InstallError。
- ProcessStartError。
- ProcessExitError。
- ControllerUnavailable。
- AuthenticationError。
- ConfigParseError。
- ConfigValidationError。
- ProfileUpdateError。
- PermissionError。
- UnsupportedPlatform。
- UnsupportedFeature。

用户界面展示可理解的错误原因和建议操作；调试日志保留必要上下文但脱敏。

## 12. 架构验收

- TUI 和 CLI 调用相同的应用服务。
- TUI 不直接调用 Mihomo API。
- Proxy Manager 不直接管理操作系统进程。
- Core Manager 不包含 UI 逻辑。
- 代理流量不经过 Termihomo。
- 退出 TUI 不停止 Mihomo。
- API 不健康不等同于进程已退出。
- 配置更新具备校验、备份、原子替换和失败恢复。
- 平台特定代码有明确边界。
- 首版不引入未经验证的 IPC 假设。
