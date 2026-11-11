package platform

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/Nyakooo/termihomo/internal/i18n"
)

type Paths struct {
	Config string
	Data   string
	Logs   string
}

func ResolvePaths() (Paths, error) {
	if runtime.GOOS != "linux" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		return Paths{}, i18n.Message("supported platforms: Linux AMD64 and ARM64")
	}
	home, err := os.UserHomeDir()
	if err != nil || !filepath.IsAbs(home) {
		return Paths{}, i18n.Message("HOME must be an absolute path")
	}
	base := func(key, fallback string) string {
		if path := os.Getenv(key); filepath.IsAbs(path) {
			return filepath.Join(path, "termihomo")
		}
		return filepath.Join(home, fallback, "termihomo")
	}
	return Paths{
		Config: base("XDG_CONFIG_HOME", ".config"),
		Data:   base("XDG_DATA_HOME", ".local/share"),
		Logs:   filepath.Join(base("XDG_STATE_HOME", ".local/state"), "logs"),
	}, nil
}

func (p Paths) Ensure() error {
	for _, path := range []string{p.Config, p.Data, filepath.Dir(p.Logs), p.Logs} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
			return i18n.Message("application directories must be private directories")
		}
	}
	return nil
}
