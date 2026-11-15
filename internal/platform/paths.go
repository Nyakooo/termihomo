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
	desktop := runtime.GOOS == "linux" || runtime.GOOS == "darwin" || runtime.GOOS == "windows"
	if !(desktop && (runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64") ||
		runtime.GOOS == "android" && runtime.GOARCH == "arm64") {
		return Paths{}, i18n.Message("supported platforms: Linux, macOS, and Windows (AMD64 and ARM64), Android (ARM64)")
	}
	home, err := os.UserHomeDir()
	if err != nil || !filepath.IsAbs(home) {
		return Paths{}, i18n.Message("HOME must be an absolute path")
	}
	config, data, state := defaultDirectories(home)
	base := func(key, fallback string) string {
		if path := os.Getenv(key); filepath.IsAbs(path) {
			return filepath.Join(path, "termihomo")
		}
		return fallback
	}
	return Paths{
		Config: base("XDG_CONFIG_HOME", config),
		Data:   base("XDG_DATA_HOME", data),
		Logs:   filepath.Join(base("XDG_STATE_HOME", state), "logs"),
	}, nil
}

func (p Paths) Ensure() error {
	for _, path := range []string{p.Config, p.Data, filepath.Dir(p.Logs), p.Logs} {
		if err := ensurePrivateDirectory(path); err != nil {
			return err
		}
	}
	return nil
}

func defaultDirectories(home string) (config, data, state string) {
	switch runtime.GOOS {
	case "darwin":
		base := filepath.Join(home, "Library", "Application Support", "termihomo")
		return filepath.Join(base, "config"), filepath.Join(base, "data"), filepath.Join(base, "state")
	case "windows":
		base := os.Getenv("LOCALAPPDATA")
		if !filepath.IsAbs(base) {
			base = filepath.Join(home, "AppData", "Local")
		}
		base = filepath.Join(base, "termihomo")
		return filepath.Join(base, "config"), filepath.Join(base, "data"), filepath.Join(base, "state")
	default:
		return filepath.Join(home, ".config", "termihomo"), filepath.Join(home, ".local", "share", "termihomo"),
			filepath.Join(home, ".local", "state", "termihomo")
	}
}
