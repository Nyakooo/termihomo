package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/Nyakooo/termihomo/internal/core"
	"github.com/Nyakooo/termihomo/internal/i18n"
	"github.com/Nyakooo/termihomo/internal/platform"
	"github.com/Nyakooo/termihomo/internal/tray"
	"github.com/Nyakooo/termihomo/internal/tui"
	"github.com/Nyakooo/termihomo/internal/update"
	"github.com/Nyakooo/termihomo/internal/version"
	"github.com/charmbracelet/x/term"
)

const help = `Usage: termihomo [--lang en|zh-CN] [command]

Commands:
  init       Initialize private configuration, data and log directories
  install    Download and verify the supported Mihomo release
  update     Update the Termihomo executable
  uninstall  Remove Termihomo and its runtime files, keeping user configuration
  start      Start the managed Mihomo in the background
  stop       Stop the managed Mihomo
  reload     Reload the current configuration in the running process
  restart    Restart the managed Mihomo process
  status [--json]  Show process and controller state
  tun start|stop     Enable or disable TUN on the running core
  profile list        Show the active profile
  profile import FILE [--yes] Import a local Mihomo YAML file
  profile add URL [--yes]     Add a subscription (use - to read the URL from stdin)
  profile update [--yes]      Refresh the saved subscription
  proxy list          List proxies and policy groups
  proxy select GROUP OPTION  Select an option in a Selector group
  proxy test NAME [URL]      Measure a proxy's delay
  rules [--json]      List rules in evaluation order
  logs [-f]           Show or follow Mihomo logs
  config check        Validate the managed configuration
  config backup       Back up the generated configuration
  version    Show the Termihomo version
  help       Show this help
`

const helpZH = `用法：termihomo [--lang en|zh-CN] [命令]

命令：
  init                        初始化私有配置、数据和日志目录
  install                     下载并校验支持的 Mihomo 版本
  update                      更新 Termihomo 程序
  uninstall                   卸载程序并清理运行文件，保留用户配置
  start                       在后台启动托管的 Mihomo
  stop                        停止内核
  reload                      在当前进程中重新加载配置
  restart                     重启内核进程
  status [--json]             查看进程和控制接口状态
  tun start|stop              开启或关闭运行中内核的 TUN
  profile list                查看当前配置
  profile import FILE [--yes] 导入本地 Mihomo YAML
  profile add URL [--yes]     添加订阅（用 - 从标准输入读取地址）
  profile update [--yes]      刷新订阅
  proxy list                  列出节点和策略组
  proxy select GROUP OPTION   在 Selector 策略组中选择选项
  proxy test NAME [URL]        测试节点延迟
  rules [--json]              按匹配顺序列出规则
  logs [-f]                   查看或跟踪日志
  config check                校验托管配置
  config backup               备份生成的配置
  version                     显示版本
  help                        显示帮助
`

func RunWithInput(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if runtime.GOOS == "windows" && len(args) > 0 && args[0] == tray.ConsoleArg {
		return runTrayConsole(args[1:], stderr)
	}
	originalArgs := args
	lang := i18n.FromEnvironment()
	if _, explicit := i18n.Parse(os.Getenv("TERMIHOMO_LANG")); !explicit {
		if paths, err := platform.ResolvePaths(); err == nil {
			lang = i18n.Saved(paths.Config)
		}
	}
	if len(args) > 0 && args[0] == "--lang" {
		var requested i18n.Language
		var ok bool
		if len(args) >= 2 {
			requested, ok = i18n.Parse(args[1])
		}
		if !ok {
			fmt.Fprintln(stderr, lang.Text("use --lang en or --lang zh-CN"))
			return 2
		}
		lang = requested
		args = args[2:]
	}

	showNodes := len(args) == 1 && args[0] == tray.NodesArg
	if len(args) == 0 || showNodes {
		if handled, code := bridgeTerminal(originalArgs, stdin, stdout, stderr, lang); handled {
			return code
		}
	}
	if (len(args) == 0 || showNodes) && isTerminal(stdin) && isTerminal(stdout) {
		paths, err := platform.ResolvePaths()
		if err == nil {
			err = tui.Run(core.Manager{Paths: paths}, stdin, stdout, lang, showNodes)
		}
		if err != nil {
			fmt.Fprintln(stderr, lang.Error(err))
			return 1
		}
		return 0
	}
	switch {
	case len(args) == 0 || (len(args) == 1 && (args[0] == "help" || args[0] == "--help" || args[0] == "-h")):
		if lang == i18n.Chinese {
			fmt.Fprint(stdout, helpZH)
		} else {
			fmt.Fprint(stdout, help)
		}
		return 0
	case len(args) == 1 && args[0] == "version":
		fmt.Fprintf(stdout, "termihomo %s (%s/%s)\n", version.Value, runtime.GOOS, runtime.GOARCH)
		return 0
	case len(args) == 1 && args[0] == "uninstall":
		scheduled, err := uninstall()
		if err != nil {
			fmt.Fprintln(stderr, lang.Error(err))
			return 1
		}
		message := "Termihomo uninstalled. User configuration was kept."
		if scheduled {
			message = "Termihomo will be removed after this process exits. User configuration was kept."
		}
		fmt.Fprintln(stdout, lang.Text(message))
		return 0
	case len(args) == 1 && args[0] == "update":
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		tag, changed, err := update.Run(ctx, version.Value)
		if errors.Is(err, update.ErrUpdateScheduled) {
			fmt.Fprintf(stdout, lang.Text("Termihomo %s will be installed after this process exits.")+"\n", tag)
			return 0
		}
		if err != nil {
			fmt.Fprintln(stderr, lang.Error(err))
			return 1
		}
		if changed {
			fmt.Fprintf(stdout, lang.Text("Updated Termihomo to %s")+"\n", tag)
		} else {
			fmt.Fprintf(stdout, lang.Text("Termihomo %s is up to date")+"\n", tag)
		}
		return 0
	case len(args) == 1 && args[0] == "init":
		paths, err := platform.ResolvePaths()
		if err == nil {
			err = paths.Ensure()
		}
		// Keep raw errors, paths and user arguments out of structured logs.
		log := slog.New(slog.NewJSONHandler(stderr, nil))
		if err != nil {
			log.Error(lang.Text("could not initialize private application directories"), "operation", "init", "hint", lang.Text("check HOME, XDG paths and directory permissions"))
			return 1
		}
		log.Info(lang.Text("application directories ready"), "operation", "init", "os", runtime.GOOS, "arch", runtime.GOARCH)
		return 0
	default:
		return runCore(args, stdin, stdout, stderr, lang)
	}
}

func runTrayConsole(args []string, stderr io.Writer) int {
	input, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer input.Close()
	output, err := os.OpenFile("CONOUT$", os.O_RDWR, 0)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer output.Close()
	return RunWithInput(args, input, output, output)
}

func runCore(args []string, stdin io.Reader, stdout, stderr io.Writer, lang i18n.Language) int {
	var paths platform.Paths
	bootStart := len(args) == 4 && args[0] == "--boot-start"
	if bootStart {
		paths = platform.Paths{Config: args[1], Data: args[2], Logs: args[3]}
		if !filepath.IsAbs(paths.Config) || !filepath.IsAbs(paths.Data) || !filepath.IsAbs(paths.Logs) {
			fmt.Fprintln(stderr, lang.Text("Startup directories must be absolute paths"))
			return 2
		}
		args = []string{"start"}
	}
	confirmed := len(args) >= 2 && args[len(args)-1] == "--yes"
	if confirmed {
		args = args[:len(args)-1]
	}
	mayConfirm := args[0] == "stop" || args[0] == "restart" ||
		(args[0] == "profile" && len(args) >= 2 && args[1] != "list")
	if !validManagedCommand(args) || (confirmed && !mayConfirm) {
		fmt.Fprintln(stderr, lang.Text("unknown command or arguments; run termihomo --help"))
		return 2
	}

	var err error
	if paths.Config == "" {
		paths, err = platform.ResolvePaths()
		if err != nil {
			fmt.Fprintln(stderr, lang.Error(err))
			return 1
		}
	}
	manager := core.Manager{Paths: paths}
	needsConfirmation := args[0] == "profile" && mayConfirm
	if args[0] == "profile" && mayConfirm && args[1] != "update" {
		info, err := manager.Profile()
		if err != nil {
			fmt.Fprintln(stderr, lang.Error(err))
			return 1
		}
		needsConfirmation = info.Source != ""
	}
	if needsConfirmation && !confirmed {
		if !isTerminal(stdin) && !isMSYSTerminal(stdin) {
			fmt.Fprintln(stderr, lang.Text("confirmation required; use --yes for non-interactive execution"))
			return 2
		}
		fmt.Fprint(stderr, lang.Text("Apply this change? [y/N] "))
		answer, _ := bufio.NewReader(stdin).ReadString('\n')
		if !strings.EqualFold(strings.TrimSpace(answer), "y") {
			return 0
		}
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	switch args[0] {
	case "install":
		err = manager.Install(ctx)
		if err == nil {
			fmt.Fprintf(stdout, lang.Text("Mihomo %s installed\n"), core.Version)
		}
	case "start":
		err = manager.Start(ctx)
	case "stop":
		err = manager.Stop()
	case "reload":
		err = manager.Reload(ctx)
		if err == nil {
			fmt.Fprintln(stdout, lang.Text("Mihomo configuration reloaded"))
		}
	case "restart":
		err = manager.Restart(ctx)
	case "tun":
		enable := args[1] == "start"
		err = manager.SetTUN(ctx, enable)
		if err == nil {
			message := "TUN disabled"
			if enable {
				message = "TUN enabled"
			}
			fmt.Fprintln(stdout, lang.Text(message))
		}
	case "logs":
		err = runLogs(ctx, manager, len(args) == 2, stdout, stderr, lang)
	case "config":
		if args[1] == "check" {
			err = manager.CheckConfig(ctx)
		} else {
			var backup string
			backup, err = manager.BackupConfig()
			if err == nil {
				fmt.Fprintln(stdout, backup)
			}
		}
	case "profile":
		err = runProfile(ctx, manager, args[1:], stdin, stdout, lang)
	case "proxy":
		err = runProxy(ctx, manager, args[1:], stdout)
	case "rules":
		var rules []core.Rule
		rules, err = manager.Rules(ctx)
		if err == nil {
			if len(args) == 2 {
				err = json.NewEncoder(stdout).Encode(rules)
			} else {
				for _, rule := range rules {
					fmt.Fprintf(stdout, "%d\t%q\t%q\t%q\n", rule.Index, rule.Type, rule.Payload, rule.Proxy)
				}
			}
		}
	case "status":
		var status core.Status
		status, err = manager.Status(ctx)
		if err == nil {
			if len(args) == 2 {
				err = json.NewEncoder(stdout).Encode(status)
			} else {
				fmt.Fprint(stdout, lang.Text(status.State))
				if status.Installed != "" {
					fmt.Fprint(stdout, " · Mihomo ", status.Installed)
				}
				if status.PID != 0 {
					fmt.Fprintf(stdout, " · PID %d · %s", status.PID, status.Controller)
					fmt.Fprint(stdout, lang.Text(" · Proxy "), status.Proxy)
				}
				fmt.Fprintln(stdout)
			}
		}
	}
	if err == nil && !bootStart && (args[0] == "start" || args[0] == "restart") && manager.TrayEnabled() && tray.Available() {
		err = tray.Start(paths)
	}
	if err != nil {
		fmt.Fprintln(stderr, lang.Error(err))
		return 1
	}
	return 0
}

func validManagedCommand(args []string) bool {
	switch args[0] {
	case "install", "start", "stop", "reload", "restart":
		return len(args) == 1
	case "tun":
		return len(args) == 2 && (args[1] == "start" || args[1] == "stop")
	case "status":
		return len(args) == 1 || (len(args) == 2 && args[1] == "--json")
	case "rules":
		return len(args) == 1 || (len(args) == 2 && args[1] == "--json")
	case "logs":
		return len(args) == 1 || (len(args) == 2 && args[1] == "-f")
	case "config":
		return len(args) == 2 && (args[1] == "check" || args[1] == "backup")
	case "profile":
		if len(args) < 2 {
			return false
		}
		switch args[1] {
		case "list", "update":
			return len(args) == 2
		case "import", "add":
			return len(args) == 3
		}
	case "proxy":
		if len(args) < 2 {
			return false
		}
		switch args[1] {
		case "list":
			return len(args) == 2
		case "select":
			return len(args) == 4
		case "test":
			return len(args) == 3 || len(args) == 4
		}
	}
	return false
}

func runProfile(ctx context.Context, manager core.Manager, args []string, stdin io.Reader, stdout io.Writer, lang i18n.Language) error {
	switch args[0] {
	case "import":
		return manager.ImportProfile(ctx, args[1])
	case "add":
		source := args[1]
		if source == "-" {
			const limit = 16 << 10
			body, err := io.ReadAll(io.LimitReader(stdin, limit+1))
			if err != nil || len(body) > limit {
				return i18n.Message("could not read subscription URL from stdin")
			}
			source = strings.TrimSpace(string(body))
		}
		return manager.AddSubscription(ctx, source)
	case "update":
		return manager.UpdateProfile(ctx)
	}
	info, err := manager.Profile()
	if err != nil {
		return err
	}
	if info.Source == "" {
		fmt.Fprintln(stdout, lang.Text("No profile configured"))
	} else {
		fmt.Fprintf(stdout, lang.Text("Active profile · %s · updated %s\n"), lang.Text(info.Source), info.Updated.Format("2006-01-02 15:04:05"))
	}
	return nil
}

func runProxy(ctx context.Context, manager core.Manager, args []string, stdout io.Writer) error {
	switch args[0] {
	case "select":
		return manager.Select(ctx, args[1], args[2])
	case "test":
		target := ""
		if len(args) == 3 {
			target = args[2]
		}
		delay, err := manager.Delay(ctx, args[1], target)
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "%d ms\n", delay)
	default:
		proxies, err := manager.Proxies(ctx)
		if err != nil {
			return err
		}
		for _, p := range proxies {
			fmt.Fprintf(stdout, "%q\t%s", p.Name, p.Type)
			if p.Now != "" {
				fmt.Fprintf(stdout, "\t%q", p.Now)
			}
			fmt.Fprintln(stdout)
		}
	}
	return nil
}

func runLogs(ctx context.Context, manager core.Manager, follow bool, stdout, stderr io.Writer, lang i18n.Language) error {
	lines, err := manager.RecentLogs()
	if err != nil {
		return err
	}
	for _, line := range lines {
		timestamp, event, found := strings.Cut(line, " ")
		if found {
			line = timestamp + " " + lang.Text(event)
		}
		fmt.Fprintln(stdout, "[termihomo]", line)
	}
	if !follow {
		return nil
	}
	for ctx.Err() == nil {
		err := manager.StreamLogs(ctx, "info", func(entry core.LogEntry) {
			fmt.Fprintf(stdout, "[mihomo %s] %s\n", lang.Text(entry.Type), entry.Payload)
		})
		if ctx.Err() != nil {
			break
		}
		fmt.Fprintln(stderr, lang.Error(err), lang.Text("· retrying"))
		select {
		case <-ctx.Done():
		case <-time.After(2 * time.Second):
		}
	}
	return nil
}

func isTerminal(stream any) bool {
	file, ok := stream.(interface{ Fd() uintptr })
	return ok && term.IsTerminal(file.Fd())
}
