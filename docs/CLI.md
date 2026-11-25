# CLI 命令参考

Termihomo 的 CLI 与 TUI 共用同一套管理逻辑。适合远程管理、脚本调用和不需要交互界面的场景。

先查看当前构建支持的命令：

```sh
termihomo help
```

## 全局选项

| 选项 | 说明 |
| --- | --- |
| `--lang en` | 使用英文输出和界面 |
| `--lang zh-CN` | 使用简体中文输出和界面 |
| `--help`、`-h` | 显示帮助 |

语言选项放在命令前：

```sh
termihomo --lang en status
termihomo --lang zh-CN
```

## 程序与内核

| 命令 | 说明 |
| --- | --- |
| `termihomo` | 在交互式终端中启动 TUI |
| `termihomo init` | 初始化应用配置、数据和日志目录 |
| `termihomo install` | 下载并校验受支持的 Mihomo 内核 |
| `termihomo update` | 更新 Termihomo 可执行程序，显示检查、下载、校验和安装进度 |
| `termihomo uninstall` | 卸载程序及运行数据，保留配置 |
| `termihomo version` | 显示版本和当前平台 |

耗时命令（安装、启动/重启、重载、配置检查、订阅导入/更新、测速、停止和卸载）会向 stderr 输出阶段或开始提示，成功后显示完成结果。`update` 在交互式终端中显示下载进度条；重定向时只输出阶段提示。查询命令和 JSON 输出保持简洁。

### 卸载

`termihomo uninstall` 会退出托盘和终端界面，恢复 Termihomo 接管的系统代理，移除开机启动项并停止托管的 Mihomo。随后清理当前可执行文件、内核、日志和缓存，保留订阅、配置档案、应用设置及 `termihomo config backup` 生成的手动备份。Windows 在命令进程退出后移除可执行文件。

### 内核生命周期

```sh
termihomo start
termihomo status
termihomo reload
termihomo restart
termihomo stop
termihomo down
```

- `start`：启动由 Termihomo 管理的 Mihomo 后台进程；若已启用系统托盘且当前桌面支持托盘，同时补启托盘。
- `status`：查看进程与 Controller 状态。
- `reload`：在当前进程中重新加载配置。端口等启动参数变更可能需要重启。
- `restart`：停止并重新启动内核，现有连接会中断；同样按系统托盘设置补启托盘，已有托盘不会重复启动。
- `stop`：停止托管的内核进程，恢复 Termihomo 接管的系统代理；保留托盘和终端界面，托盘会刷新为停止状态。
- `down`：退出托盘和终端界面、停止内核并恢复接管的系统代理；保留配置、托盘开关与开机启动设置。

状态支持 JSON 输出：

```sh
termihomo status --json
```

### TUN

```sh
termihomo tun start
termihomo tun stop
termihomo tun status
termihomo tun status --json
```

开启 TUN 通常需要系统网络管理权限。请先确认当前操作系统和运行环境满足 Mihomo 的 TUN 要求。

### 系统代理

```sh
termihomo sysproxy start
termihomo sysproxy stop
termihomo sysproxy status
termihomo sysproxy status --json
```

开启系统代理要求内核运行。关闭默认只恢复 Termihomo 接管的设置；如果当前代理由其他程序设置，需要交互确认，非交互执行使用 `termihomo sysproxy stop --yes`。确认后会校验代理设置是否变化，再关闭外部代理。`sysproxy start` 不会覆盖外部代理。

### 路由模式

```sh
termihomo mode rule
termihomo mode global
termihomo mode direct
termihomo mode status
termihomo mode status --json
```

切换和查询当前路由模式要求内核控制接口可用。TUN、系统代理和路由模式操作后，托盘和终端界面会自动刷新。JSON 状态字段不随界面语言变化：TUN 返回 `state`、`available`、`enabled`、`configured`、`paused`；系统代理返回 `available`、`enabled`、`owned`；路由模式返回 `mode`。系统代理和 TUN 的状态查询在不可用时仍返回 JSON，并将 `available` 设为 `false`。

### 平台与脚本说明

- 系统代理：Windows 操作当前用户的系统代理；macOS 操作当前网络服务；Linux 目前支持 GNOME 桌面代理设置；Android 不支持此系统代理接口。
- TUN：Linux、macOS 和 Windows 沿用内核的网络管理权限与授权要求；Android 需要 VPN 应用，命令不会绕过平台权限。
- `tun status --json` 中的 `available` 表示当前内核和权限下的可用性，不是对操作系统永久能力的判断；`paused` 表示暂停时保留的 TUN 恢复意图。执行 `tun stop` 后会清除恢复意图。
- `down` 等待界面退出最多 10 秒；即使界面退出失败，仍会尝试停止内核。界面退出或系统代理恢复失败会返回非零退出码，不会静默宣称完成。
- 查询当前路由模式需要可用的控制接口；平台不支持系统代理或内核未运行时，对应状态查询会报告不可用。配置损坏、权限和通信错误仍可能导致查询失败。
- 退出码：`0` 表示成功（交互确认选择取消也返回 `0`）；`1` 表示操作失败或外部代理缺少确认；`2` 表示参数错误。`--json` 只用于 `status`，系统代理命令仅 `sysproxy stop` 接受 `--yes`，用于外部代理确认。

JSON 示例（示例值，不表示当前机器状态）：

```json
{"state":"Paused","available":true,"enabled":false,"configured":false,"paused":true}
{"available":true,"enabled":true,"owned":false}
{"mode":"rule"}
```

三行依次对应 `tun status --json`、`sysproxy status --json` 和 `mode status --json`，每条命令各输出一个 JSON 对象。

## 配置档案

| 命令 | 说明 |
| --- | --- |
| `termihomo profile list` | 查看当前活动配置档案 |
| `termihomo profile add URL` | 添加并应用订阅 |
| `termihomo profile import FILE` | 导入并应用本地 YAML 配置 |
| `termihomo profile update` | 刷新当前订阅 |

示例：

```sh
termihomo profile add "https://example.com/your-subscription"
termihomo profile import ./config.yaml
termihomo profile update
```

订阅地址可能包含个人凭据，不要将真实订阅 URL 写入公开脚本、Issue 或日志。

对于非交互执行，可以在支持的变更命令后添加 `--yes`，跳过确认提示：

```sh
termihomo profile update --yes
termihomo profile import ./config.yaml --yes
```

如果操作会替换当前活动配置，建议先备份重要配置。

## 节点与策略组

| 命令 | 说明 |
| --- | --- |
| `termihomo proxy list` | 列出代理节点和策略组 |
| `termihomo proxy select GROUP OPTION` | 在 Selector 类型策略组中选择选项 |
| `termihomo proxy test NAME [URL]` | 测试指定节点的延迟 |

示例：

```sh
termihomo proxy list
termihomo proxy select "节点选择" "香港节点"
termihomo proxy test "香港节点"
```

策略组名称和选项名称必须与当前配置中的名称一致。只有 Selector 类型的策略组支持通过该命令直接选择。

## 规则与日志

```sh
termihomo rules
termihomo rules --json
termihomo logs
termihomo logs -f
```

- `rules` 按匹配顺序列出当前规则。
- `rules --json` 输出 JSON，便于脚本解析。
- `logs` 查看最近的 Mihomo 日志。
- `logs -f` 持续跟踪新日志。

## 配置检查与备份

```sh
termihomo config check
termihomo config backup
```

- `config check` 使用 Mihomo 检查当前托管配置。
- `config backup` 备份当前生成的运行配置，并输出备份文件位置。

备份命令针对生成的运行配置，不等同于导出全部配置档案。档案和应用数据的存储位置见[配置与数据目录](CONFIGURATION.md)。

## 退出状态

CLI 命令成功时返回退出码 `0`；参数错误、执行失败或校验失败时返回非零退出码。脚本可使用进程退出状态判断命令是否成功。
