package core

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/Nyakooo/termihomo/internal/platform"
)

func TestApplyNetworkPreferencesIncludesExplicitDisabledValues(t *testing.T) {
	for mask := 0; mask < 8; mask++ {
		t.Run(fmt.Sprint(mask), func(t *testing.T) {
			root := t.TempDir()
			m := Manager{Paths: platform.Paths{Config: filepath.Join(root, "config"), Data: filepath.Join(root, "data"), Logs: filepath.Join(root, "state", "logs")}}
			if err := m.Paths.Ensure(); err != nil {
				t.Fatal(err)
			}
			lan, tun, proxy := mask&1 != 0, mask&2 != 0, mask&4 != 0
			state := instance{ResumeLAN: !lan, ResumeTUN: !tun, ResumeSystemProxy: !proxy}
			for key, value := range map[string]bool{lanPreference: lan, tunPreference: tun, systemProxyPreference: proxy} {
				if err := m.saveNetworkPreference(key, value); err != nil {
					t.Fatal(err)
				}
			}
			if err := m.loadNetworkPreferences(&state); err != nil {
				t.Fatal(err)
			}
			if state.ResumeLAN != lan || state.ResumeTUN != tun || state.ResumeSystemProxy != proxy {
				t.Fatalf("saved flags not restored: %+v", state)
			}
		})
	}
}
