# Contributing

感谢你愿意为 Termihomo 提交反馈或代码。无论是问题报告、文档改进还是代码贡献，都欢迎参与。

Thank you for your interest in contributing to Termihomo. Bug reports, documentation improvements, and code contributions are welcome.

## 开始之前 / Before you start

- 先搜索现有 Issues，避免重复报告。
- 对于较大的功能调整，建议先开 Issue 讨论目标和实现边界。
- 不要在 Issue、Pull Request 或测试日志中公开真实订阅地址、节点凭据或个人配置。
- Keep credentials, subscription URLs, and private configuration out of public Issues, pull requests, and test logs.

## 开发环境 / Development setup

项目使用 Go。请使用与仓库 `go.mod` 声明一致的 Go 工具链。

The project is written in Go. Use the Go toolchain version declared in `go.mod`.

```sh
git clone https://github.com/Nyakooo/termihomo.git
cd termihomo
go test ./...
go build ./cmd/termihomo
```

## 代码约定 / Code guidelines

- 保持改动聚焦，避免无关重构。
- 新功能优先放在对应的 Core Manager 能力中，再接入 TUI 或 CLI。
- 平台专属实现应使用合适的 Go build tags，避免污染通用代码。
- 用户可见文本应接入现有 i18n 机制，并同时考虑简体中文和英文。
- 为关键逻辑补充测试；修复 Bug 时尽量添加可复现的回归测试。
- Run `gofmt` on changed Go files and add tests for behavior changes.
- Keep pull requests focused and explain the motivation, behavior change, and verification steps.

## 提交前检查 / Before submitting

```sh
gofmt -w <changed-go-files>
go test ./...
go build ./cmd/termihomo
```

请在 Pull Request 中说明：

1. 修改解决了什么问题。
2. 是否改变用户可见行为、配置格式或命令参数。
3. 在哪些操作系统和架构上进行了验证。
4. 相关测试的执行结果。

A pull request should describe the problem, user-visible changes, compatibility impact, tested platforms, and test results.

## 报告问题 / Reporting bugs

提交问题时请尽量提供：

- Termihomo 版本和操作系统 / 架构。
- 复现步骤和预期结果、实际结果。
- 相关错误信息或经过脱敏的日志。
- 如果涉及系统代理或 TUN，请说明运行权限和网络环境。

Do not attach unredacted profiles or logs. Remove subscription URLs, passwords, tokens, and private addresses before sharing diagnostic information.
