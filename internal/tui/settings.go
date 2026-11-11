package tui

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"go.yaml.in/yaml/v3"
)

type settings struct {
	RefreshSeconds int               `yaml:"refresh-seconds"`
	Keys           map[string]string `yaml:"keys"`
}

var defaultKeys = map[string]string{
	"quit": "q", "help": "?", "refresh": "r", "search": "/", "sort": "s",
	"test": "t", "log-level": "l",
}

func loadSettings(dir string) (settings, error) {
	config := settings{RefreshSeconds: 3}
	body, err := os.ReadFile(filepath.Join(dir, "settings.yaml"))
	if errors.Is(err, os.ErrNotExist) {
		return config, nil
	}
	if err != nil {
		return config, err
	}
	if err := yaml.Unmarshal(body, &config); err != nil {
		return config, errors.New("invalid Termihomo settings YAML")
	}
	if config.RefreshSeconds < 1 || config.RefreshSeconds > 300 {
		return config, errors.New("refresh-seconds must be between 1 and 300")
	}
	used := map[string]bool{
		"tab": true, "shift+tab": true, "left": true, "right": true,
		"up": true, "down": true, "j": true, "k": true, "enter": true, "esc": true,
		"ctrl+c": true, "i": true, "a": true, "x": true, "R": true, "u": true,
		"1": true, "2": true, "3": true, "4": true, "5": true, "6": true, "7": true,
	}
	for action := range config.Keys {
		if _, ok := defaultKeys[action]; !ok {
			return config, errors.New("settings contain an unknown shortcut action")
		}
	}
	for action := range defaultKeys {
		key := config.key(action)
		if used[key] {
			return config, errors.New("settings contain a duplicate or reserved shortcut")
		}
		used[key] = true
	}
	return config, nil
}

func (s settings) key(action string) string {
	if key := s.Keys[action]; key != "" {
		return key
	}
	return defaultKeys[action]
}

func (s settings) interval() time.Duration { return time.Duration(s.RefreshSeconds) * time.Second }
