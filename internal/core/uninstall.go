package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func (m Manager) Uninstall(ctx context.Context) error {
	return m.withLock(func() error {
		if err := m.removeBootRegistration(ctx); err != nil {
			return err
		}
		if err := m.syncDesktopStartup(false); err != nil {
			return err
		}
		owner, _ := m.proxyOwnership()
		if len(owner.Applied) > 0 {
			if err := m.disableSystemProxy(ctx, ""); err != nil {
				return err
			}
		}
		if err := m.stop(); err != nil {
			return err
		}
		return m.clearRuntimeFiles()
	})
}

func (m Manager) clearRuntimeFiles() error {
	config, err := filepath.EvalSymlinks(m.Paths.Config)
	if err != nil {
		return err
	}
	remove := func(path string) error {
		// Resolve parents to protect configurations in overlapping XDG trees.
		parent, err := filepath.EvalSymlinks(filepath.Dir(path))
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(filepath.Join(parent, filepath.Base(path)), config)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return nil
		}
		return os.RemoveAll(path)
	}

	if info, err := os.Lstat(m.runDir()); err == nil && info.IsDir() {
		generated := []string{"config.yaml", "config.yaml.bak", "cache.db"}
		files, err := geodataFiles()
		if err != nil {
			return err
		}
		for _, file := range files {
			generated = append(generated, file.Name)
		}
		for _, name := range generated {
			if err := remove(filepath.Join(m.runDir(), name)); err != nil {
				return err
			}
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, name := range []string{
		"cores", "geodata.json", "instance.json", "profile-change.json", "system-proxy.json",
		"tray-worker.exe", "tray-exit", "tray-nodes",
	} {
		if err := remove(filepath.Join(m.Paths.Data, name)); err != nil {
			return err
		}
	}
	if err := remove(m.Paths.Logs); err != nil {
		return err
	}
	for _, dir := range []string{m.runDir(), m.Paths.Data, filepath.Dir(m.Paths.Logs)} {
		resolved, err := filepath.EvalSymlinks(dir)
		if err != nil || resolved == config {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err == nil && len(entries) == 0 {
			_ = os.Remove(dir)
		}
	}
	return nil
}
