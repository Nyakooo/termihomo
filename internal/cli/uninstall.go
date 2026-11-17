package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Nyakooo/termihomo/internal/core"
	"github.com/Nyakooo/termihomo/internal/platform"
	"github.com/Nyakooo/termihomo/internal/tray"
	"github.com/Nyakooo/termihomo/internal/update"
)

func uninstall() (bool, error) {
	executable, err := os.Executable()
	if err != nil {
		return false, err
	}
	lock, err := platform.LockExecutableUpdate(executable)
	if err != nil {
		return false, err
	}
	defer lock.Close()
	paths, err := platform.ResolvePaths()
	if err != nil {
		return false, err
	}
	if err := paths.Ensure(); err != nil {
		return false, err
	}
	ctx, done := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer done()
	shutdown, cancel := context.WithTimeout(ctx, 10*time.Second)
	err = tray.Shutdown(shutdown, paths)
	cancel()
	if err != nil {
		return false, err
	}
	if err := (core.Manager{Paths: paths}).Uninstall(ctx); err != nil {
		return false, err
	}
	return update.RemoveExecutable(executable)
}
