# 配置与数据目录

本文说明 Termihomo 在本机保存哪些数据、各平台默认目录在哪里，以及哪些文件适合手动调整。

## 目录布局

Termihomo 将配置、运行数据和日志分开存放。

| 平台 | 配置目录 | 数据目录 | 日志目录 |
| --- | --- | --- | --- |
| Linux | `~/.config/termihomo` | `~/.local/share/termihomo` | `~/.local/state/termihomo/logs` |
| macOS | `~/Library/Application Support/termihomo/config` | `~/Library/Application Support/termihomo/data` | `~/Library/Application Support/termihomo/state/logs` |
| Windows | `%LOCALAPPDATA%\termihomo\config` | `%LOCALAPPDATA%\termihomo\data` | `%LOCALAPPDATA%\termihomo\state\logs` |

Linux 支持 XDG Base Directory 环境变量。设置为绝对路径时，Termihomo 会在对应目录下创建 `termihomo` 子目录：

- `XDG_CONFIG_HOME`
- `XDG_DATA_HOME`
- `XDG_STATE_HOME`

## 重要文件

### 配置目录

| 路径 | 用途 |
| --- | --- |
| `profile.json` | 当前活动配置档案及其元数据 |
| `profiles/` | 已保存的配置档案库 |
| `overrides.yaml` | Termihomo 管理的配置覆盖项 |
| `settings.yaml` | TUI 刷新间隔和快捷键设置 |
| `language` | 保存的界面语言 |

`settings.yaml` 可用于调整刷新间隔和部分快捷键。例如：

~~~yaml
refresh-seconds: 3
keys:
  quit: q
  help: "?"
  refresh: r
  search: /
  sort: s
  log-level: v
~~~

快捷键不能与界面保留按键冲突；无效设置会在启动 TUI 时报告。

### 常规开关

在 **工具 → 常规**中按回车切换布尔选项。

| 设置 | 行为 |
| --- | --- |
| 自动运行 | 打开 Termihomo 界面时自动启动代理 |
| 开机启动 | 开机后启动托管内核。Windows 注册登录前启动任务需要管理员权限 |
| 切换节点时关闭连接 | 手动切换节点后关闭 Mihomo 管理的所有活动连接，默认关闭 |
| 系统托盘 | 有托盘宿主时显示常驻托盘，默认关闭 |

系统代理、TUN 和局域网代理的开关会保存，并在开机启动、自动运行和暂停后恢复时重新应用。退出或暂停代理不会清除这些偏好；重启内核或重新加载配置也会保留局域网代理状态。

登录前无法访问桌面系统代理设置时，会在桌面登录后补做恢复，即使系统托盘关闭也会执行。登录恢复会对锁忙、内核或网络尚未就绪的情况等待重试，最长两分钟，且不会弹出 TUN 授权。开机时单项网络配置恢复失败会记录错误并保留已启动的内核。TUN 仍需要系统网络权限，局域网代理需要可用的私有 IPv4 网络；恢复失败会报告错误，且不会清除已保存的开关。

### 数据目录

| 路径 | 用途 |
| --- | --- |
| `instance.json` | 托管内核的运行状态 |
| `mihomo/config.yaml` | 根据活动档案和应用设置生成的运行配置 |
| `mihomo/config.yaml.manual.bak` | 手动运行 `termihomo config backup` 生成的备份，卸载时保留 |
| `cores/` | 已安装的 Mihomo 内核版本 |

### 日志目录

| 路径 | 用途 |
| --- | --- |
| `termihomo.log` | Termihomo 应用日志 |

Mihomo 自身的运行日志可通过 TUI 的 Logs 页面或 `termihomo logs` 查看。

## 环境变量

| 变量 | 作用 |
| --- | --- |
| `XDG_CONFIG_HOME` | Linux 配置目录的父目录 |
| `XDG_DATA_HOME` | Linux 数据目录的父目录 |
| `XDG_STATE_HOME` | Linux 状态与日志目录的父目录 |
| `TERMIHOMO_LANG` | 设置默认语言，可用值为 `en` 或 `zh-CN` |
| `TERMIHOMO_INSTALL_DIR` | 覆盖安装脚本的默认安装位置 |

命令行的 `--lang` 可为当前启动指定语言；TUI 中选择的语言会保存在配置目录中。

## 配置如何生效

Termihomo 将配置档案和应用覆盖项组合成托管配置：

1. 导入本地 Mihomo YAML，或下载订阅配置。
2. 保存活动档案。
3. 将 Termihomo 的设置覆盖到对应配置项。
4. 调用 Mihomo 校验生成结果。
5. 校验通过后再写入并启动或重新加载。

因此，`mihomo/config.yaml` 是生成的运行文件，不建议直接编辑。重新启动、重新加载或修改设置时，它可能被重新生成。

需要调整通用选项、DNS 或网络设置时，优先使用 TUI 的设置页面。配置档案本身应通过 Profile 页面或 CLI 导入、更新和管理。

## 代理默认值

- 默认混合代理监听地址：`127.0.0.1:7890`
- 默认不允许局域网访问。
- 运行配置由 Termihomo 管理；不要同时用其他工具覆盖同一份托管配置。
- 系统代理和 TUN 属于系统网络设置，启用时请留意权限与网络影响。

## 安全与备份

配置目录会以当前用户可访问的私有目录方式创建。不要随意修改目录权限，也不要将配置目录直接提交到公开仓库。

特别注意：

- 订阅 URL 可能包含账户令牌或其他访问凭据。
- 配置文件可能包含节点密码、密钥、域名和个人网络信息。
- 分享日志前先检查是否包含订阅地址、服务器地址或其他敏感信息。
- `termihomo config backup` 备份的是生成的运行配置，不等同于完整备份。需要完整迁移时，应在停止内核后，安全备份配置目录和必要的数据目录。

## 安装目录

安装程序默认将 Termihomo 可执行文件安装到：

- Linux / macOS：`~/.local/bin/termihomo`
- Windows：`%LOCALAPPDATA%\Programs\Termihomo\termihomo.exe`

设置 `TERMIHOMO_INSTALL_DIR` 可覆盖安装程序的默认目标目录。它只影响程序安装位置，不改变应用配置、数据和日志目录。
