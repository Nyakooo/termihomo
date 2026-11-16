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
| `termihomo update` | 更新 Termihomo 可执行程序 |
| `termihomo version` | 显示版本和当前平台 |

### 内核生命周期

```sh
termihomo start
termihomo status
termihomo reload
termihomo restart
termihomo stop
```

- `start`：启动由 Termihomo 管理的 Mihomo 后台进程。
- `status`：查看进程与 Controller 状态。
- `reload`：在当前进程中重新加载配置。端口等启动参数变更可能需要重启。
- `restart`：停止并重新启动内核，现有连接会中断。
- `stop`：停止托管的内核进程。

状态支持 JSON 输出：

```sh
termihomo status --json
```

### TUN

```sh
termihomo tun start
termihomo tun stop
```

开启 TUN 通常需要系统网络管理权限。请先确认当前操作系统和运行环境满足 Mihomo 的 TUN 要求。

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
