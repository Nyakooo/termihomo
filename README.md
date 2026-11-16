<p align="center">
  <img src="assets/termihomo-logo.svg" alt="Termihomo" width="180" />
</p>

<h1 align="center">Termihomo</h1>

<p align="center">
  <strong>在终端中管理 Mihomo。</strong><br />
  一个轻量、跨平台的 Mihomo TUI 与 CLI 管理工具。
</p>

<p align="center">
  <a href="README.en.md">English</a> ·
  <a href="https://github.com/Nyakooo/termihomo/releases">Releases</a> ·
  <a href="docs/CLI.md">命令参考</a> ·
  <a href="docs/ARCHITECTURE.md">架构说明</a>
</p>

<p align="center">
  <a href="https://github.com/Nyakooo/termihomo/actions/workflows/ci.yml"><img src="https://github.com/Nyakooo/termihomo/actions/workflows/ci.yml/badge.svg" alt="CI and Release" /></a>
  <a href="https://github.com/Nyakooo/termihomo/releases"><img src="https://img.shields.io/github/v/release/Nyakooo/termihomo" alt="Latest Release" /></a>
</p>

Termihomo 将 Mihomo 的日常管理带回终端。它提供接近桌面管理器的信息组织方式，同时保留终端工具直接、轻量、可脚本化的使用体验。

它<strong>不是新的代理内核</strong>：网络流量仍由 <a href="https://github.com/MetaCubeX/mihomo">Mihomo</a> 处理，Termihomo 负责内核生命周期、配置与订阅、节点和策略组、连接、DNS 以及系统网络设置的管理。

## 设计目标

- **Terminal first**：以终端为主要交互界面，不依赖桌面 GUI。
- **TUI + CLI**：交互式操作与脚本化命令共用同一套管理能力。
- **轻量与克制**：聚焦 Mihomo 管理，不重复实现代理内核。
- **跨平台**：面向 Linux、macOS 和 Windows，提供 AMD64 与 ARM64 构建。
- **可控**：配置由应用统一管理，重要变更提供确认，内核配置在应用前进行校验。

## 功能

| 模块 | 能力 |
| --- | --- |
| 内核管理 | 下载与校验 Mihomo、启动、停止、重启、状态检查、后台运行 |
| 配置与订阅 | 导入本地 YAML、添加订阅、更新订阅、管理配置档案 |
| 节点与策略组 | 查看节点、切换 Selector 策略组、测试节点延迟、查看延迟历史 |
| 连接监控 | 查看活动连接、进程、目标地址、协议、规则、出口链路和流量 |
| 流量与状态 | 实时上传/下载速率、累计流量、内存占用、运行时长 |
| DNS | DNS 设置、查询记录、手动查询与结果查看 |
| 网络设置 | 系统代理、代理排除项、TUN、网络栈和出口网卡设置 |
| 规则与日志 | 查看规则匹配顺序、实时查看 Mihomo 日志 |
| 日常管理 | 中英文界面、配置检查与备份、程序自更新 |

具体功能以当前版本为准；不同操作系统的系统代理、开机启动和 TUN 能力可能存在差异。

## 界面

Termihomo 使用分区式终端界面组织信息：

- **Overview**：内核状态、实时流量、内存、当前配置、策略组和活动连接。
- **Proxies**：节点与策略组的查看、选择和延迟测试。
- **Profile**：订阅和本地配置档案的导入、编辑、更新与管理。
- **Connections**：连接列表、过滤、排序和连接详情。
- **Tools**：通用设置、网络设置、DNS、内核信息、日志和语言。

启动后按 <code>?</code> 查看当前版本的快捷键帮助。界面会根据终端尺寸调整布局；建议使用至少 80×24 的终端窗口。

## 支持平台

| 平台 | 架构 | 安装方式 |
| --- | --- | --- |
| Linux（内核 5.3+） | AMD64、ARM64 | Shell 安装脚本 / Releases |
| macOS | AMD64、ARM64 | Shell 安装脚本 / Releases |
| Windows | AMD64、ARM64 | PowerShell 安装脚本 / Releases |

## 安装

### Linux / macOS

~~~sh
curl -fsSL https://raw.githubusercontent.com/Nyakooo/termihomo/master/install.sh | sh
~~~

默认安装到 <code>~/.local/bin</code>。如果该目录不在 <code>PATH</code> 中，将其加入当前 Shell 环境：

~~~sh
export PATH="$HOME/.local/bin:$PATH"
~~~

### Windows PowerShell

~~~powershell
& ([scriptblock]::Create((Invoke-RestMethod 'https://raw.githubusercontent.com/Nyakooo/termihomo/master/install.ps1' -ErrorAction Stop)))
~~~

默认安装到 <code>%LOCALAPPDATA%\Programs\Termihomo</code>，安装程序会将目录加入当前用户的 <code>PATH</code>。

### 自定义安装目录

运行安装脚本前设置 <code>TERMIHOMO_INSTALL_DIR</code>，即可覆盖默认安装位置。

### 手动安装

也可以前往 <a href="https://github.com/Nyakooo/termihomo/releases">Releases</a> 下载对应平台的构建产物：

1. 根据操作系统和 CPU 架构选择文件。
2. 下载同一版本的 <code>SHA256SUMS</code>。
3. 使用本机的 SHA-256 工具校验文件。
4. 解压或放置可执行文件，并确保其所在目录已加入 <code>PATH</code>。

安装脚本会自动下载对应版本，并在安装前校验 SHA-256。

## 快速开始

启动交互式界面：

~~~sh
termihomo
~~~

首次运行时，Termihomo 会检查 Mihomo 内核；尚未安装时会下载并校验受支持的版本。

建议按以下顺序完成初次配置：

1. 打开 **Profile**，添加订阅链接或导入本地 Mihomo YAML。
2. 等待配置校验通过。
3. 在 **Core** 或 Overview 中启动 Mihomo。
4. 打开 **Proxies**，选择需要使用的节点或策略组。
5. 根据需要在 **Network** 中启用系统代理或 TUN。

默认本地混合代理地址为 <code>127.0.0.1:7890</code>。局域网访问默认关闭；启用 TUN 或修改系统网络设置可能需要额外的系统权限。

## CLI

Termihomo 同时提供可脚本化的命令行接口：

~~~sh
termihomo help
termihomo status
termihomo profile list
termihomo proxy list
termihomo logs -f
~~~

常用命令：

| 命令 | 说明 |
| --- | --- |
| <code>termihomo</code> | 启动交互式 TUI |
| <code>termihomo install</code> | 下载并校验 Mihomo 内核 |
| <code>termihomo update</code> | 更新 Termihomo 程序 |
| <code>termihomo start</code> / <code>stop</code> | 启动或停止后台内核 |
| <code>termihomo restart</code> / <code>reload</code> | 重启内核或重新加载配置 |
| <code>termihomo status --json</code> | 输出机器可读的状态信息 |
| <code>termihomo profile add URL</code> | 添加订阅 |
| <code>termihomo profile import FILE</code> | 导入本地 YAML 配置 |
| <code>termihomo proxy list</code> | 列出节点和策略组 |
| <code>termihomo proxy test NAME</code> | 测试节点延迟 |
| <code>termihomo rules --json</code> | 以 JSON 输出规则 |
| <code>termihomo config check</code> | 校验当前托管配置 |

完整参数、确认行为和命令示例见 <a href="docs/CLI.md">CLI 命令参考</a>。

## 文档

| 文档 | 内容 |
| --- | --- |
| [CLI 命令参考](docs/CLI.md) | 命令、参数、示例与脚本调用 |
| [配置与数据目录](docs/CONFIGURATION.md) | 配置文件、数据文件、日志和环境变量 |
| [架构说明](docs/ARCHITECTURE.md) | 模块边界、运行流程与设计原则 |
| [贡献指南](CONTRIBUTING.md) | 本地开发、测试与提交规范 |

## 从源码构建

需要与项目 <code>go.mod</code> 一致的 Go 工具链。

~~~sh
git clone https://github.com/Nyakooo/termihomo.git
cd termihomo
go test ./...
go build -trimpath -o bin/termihomo ./cmd/termihomo
~~~

运行构建产物：

~~~sh
./bin/termihomo
~~~

Windows 下请将输出文件名改为 <code>termihomo.exe</code>。更多模块说明见 <a href="docs/ARCHITECTURE.md">架构文档</a>。

## 致谢

- <a href="https://github.com/MetaCubeX/mihomo">MetaCubeX / Mihomo</a>：代理内核与控制接口。
- <a href="https://github.com/chen08209/FlClash">chen08209 / FlClash</a>：交互布局与信息组织方面的设计参考。
- <a href="https://github.com/charmbracelet">Charmbracelet</a>：终端交互相关组件。

Termihomo 是独立的管理工具，与上述项目没有隶属或官方合作关系。

## 项目状态

Termihomo 正在持续开发中。功能、命令和配置格式可能随版本调整；使用前请查看对应版本的 Release Notes。欢迎通过 Issue 反馈问题和提出改进建议。

---

<p align="center">Built for the terminal.</p>
