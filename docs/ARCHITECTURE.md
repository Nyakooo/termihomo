# 架构说明

本文介绍 Termihomo 的模块边界、运行方式和配置管理原则。面向希望理解实现、排查问题或参与开发的读者。

## 1. 项目定位

Termihomo 是 Mihomo 的终端管理器，不实现代理协议，也不替代 Mihomo 内核。

- **Mihomo** 负责代理协议、规则匹配、DNS 转发和网络流量处理。
- **Termihomo** 负责内核安装与生命周期、配置档案、节点与策略组管理、运行状态采集，以及 TUI / CLI 交互。

这种边界让项目可以专注于管理体验，并尽量复用 Mihomo 已有的能力。

## 2. 分层结构

```text
用户
 ├─ TUI（交互式终端界面）
 └─ CLI（脚本与命令行）
          │
          ▼
     Core Manager
 ├─ Mihomo 进程管理
 ├─ 配置与订阅管理
 ├─ Controller API 封装
 ├─ 节点、策略组与连接
 ├─ DNS、规则、日志
 └─ 网络与系统代理设置
          │
          ├──────── 本地配置 / 数据 / 日志
          │
          ▼
       Mihomo Core
       代理数据平面
```

### 交互层

| 包 | 职责 |
| --- | --- |
| `cmd/termihomo` | 程序入口 |
| `internal/cli` | 命令解析、CLI 操作与 TUI 启动 |
| `internal/tui` | 终端布局、页面、输入处理、交互状态 |
| `internal/tray` | 桌面托盘进程、终端启动和托盘交互 |
| `internal/i18n` | 简体中文与英文文本 |

TUI 与 CLI 调用同一套 Core Manager，不维护两套独立的代理管理逻辑。

### 管理层

| 包 | 职责 |
| --- | --- |
| `internal/core` | Mihomo 生命周期、配置生成与校验、Controller API、配置档案、代理、连接、DNS、规则和网络设置 |
| `internal/update` | Termihomo 程序更新 |
| `internal/version` | 构建版本信息 |

Core Manager 是应用与 Mihomo 之间的管理边界。TUI 和 CLI 不直接操作 Mihomo 的内部实现。

### 平台层

`internal/platform` 负责应用目录、进程锁和平台相关路径。平台专属实现通过 Go build tags 隔离，例如系统代理、进程管理和开机启动。

## 3. 运行模型

Termihomo 将 Mihomo 作为独立的后台进程管理：

1. 通过 <code>termihomo install</code> 或 TUI 的 Core 页面安装内核；安装流程会下载并校验受支持的版本。
2. 读取当前配置档案和应用覆盖设置。
3. 生成托管配置，并调用 Mihomo 校验配置。
4. 启动 Mihomo 进程，通过本地 Controller API 获取运行状态。
5. TUI / CLI 按需读取流量、节点、策略组、连接、DNS 事件和日志。

关闭 TUI 后，Mihomo 继续运行，可通过 `termihomo stop` 停止。启用系统托盘时，独立的托盘进程继续读取内核状态。托盘使用进程锁避免重复启动界面，并通过数据目录中的请求文件通知界面打开节点页或退出。

## 4. 配置处理流程

配置来源分为两类：

- **配置档案**：来自本地 Mihomo YAML 文件或订阅地址。
- **应用设置**：来自 Termihomo 的设置界面和覆盖配置。

启动或重新加载时，Termihomo 根据当前档案和应用设置生成托管配置，再交给 Mihomo 校验。校验通过后才会写入并应用。

不要把运行目录中的生成配置当作唯一配置源。它是运行产物，后续重新生成时可能被覆盖。配置目录和文件说明见[配置与数据目录](CONFIGURATION.md)。

## 5. 平台差异

Linux、macOS 和 Windows 共用主要的 Go 代码，但系统集成功能受平台能力和权限影响：

- 系统代理的读取、设置和恢复采用平台相关实现。
- 开机启动使用各平台对应的机制。
- TUN 需要系统网络权限；具体权限要求取决于操作系统和运行环境。
- 终端尺寸、鼠标事件和终端能力可能影响 TUI 的显示与交互。

因此，跨平台构建成功不代表每一种系统网络功能在所有环境下都具有完全相同的行为。

## 6. 安全边界

- 应用配置、数据和日志目录会以当前用户可访问的私有目录方式创建。
- 默认混合代理监听地址为 `127.0.0.1:7890`，局域网访问默认关闭。
- Mihomo Controller 使用本地地址和应用生成的访问凭据。
- 启用 TUN 或修改系统代理会改变系统网络行为，应在理解影响后操作。
- 订阅地址可能包含访问凭据；分享日志或配置前应先脱敏。

## 7. 代码导航

```text
cmd/termihomo/       程序入口
internal/cli/        CLI 命令
internal/tui/        TUI 页面与交互
internal/core/       Mihomo 管理与核心业务
internal/platform/   平台适配
internal/tray/       桌面托盘
internal/update/     可执行文件更新与移除
internal/i18n/       国际化
internal/version/    版本信息
test/                安装与发布产物冒烟测试
```

如果要修改某项能力，优先在对应的 Core Manager 能力中实现，再分别接入 TUI 或 CLI，避免把业务逻辑散落在界面层。
