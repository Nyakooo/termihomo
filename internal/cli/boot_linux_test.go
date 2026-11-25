//go:build !android

package cli

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Nyakooo/termihomo/internal/core"
	"github.com/Nyakooo/termihomo/internal/platform"
)

func TestBootRetainsCoreWhenNetworkRestorationFails(t *testing.T) {
	isolateCLI(t)
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("fixture requires Python 3")
	}
	paths, err := platform.ResolvePaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	manager := core.Manager{Paths: paths}
	fixture, err := os.ReadFile("../core/testdata/network_kernel.py")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(manager.BinaryPath()), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manager.BinaryPath(), append([]byte("#!"+python+"\n"), fixture...), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_NETWORK_KERNEL_VERSION", core.Version)
	t.Setenv("TEST_NETWORK_CONFIG_FAILURE", "1")
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	settings := fmt.Sprintf("mixed-port: %d\napp.tun-enabled: true\n", port)
	if err := os.WriteFile(filepath.Join(paths.Config, "overrides.yaml"), []byte(settings), 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := manager.Stop(); err != nil {
			t.Error(err)
		}
	})
	var out, diagnostics bytes.Buffer
	args := []string{"--boot-start", paths.Config, paths.Data, paths.Logs}
	if code := runCLI(args, &out, &diagnostics); code != 0 {
		t.Fatalf("ready core would be killed by boot service: exit=%d %s", code, &diagnostics)
	}
	if diagnostics.Len() == 0 {
		t.Fatal("partial restoration failure was not reported")
	}
	status, err := manager.Status(t.Context())
	if err != nil || status.State != "Running" {
		t.Fatalf("core not running: %+v %v", status, err)
	}
	// Manual startup must still report incomplete restoration as a failure.
	diagnostics.Reset()
	if code := runCLI([]string{"start"}, &out, &diagnostics); code != 1 {
		t.Fatalf("manual start hid restoration failure: exit=%d", code)
	}
}

func TestBootStillFailsWhenCoreCannotStart(t *testing.T) {
	isolateCLI(t)
	paths, err := platform.ResolvePaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	manager := core.Manager{Paths: paths}
	if err := os.MkdirAll(filepath.Dir(manager.BinaryPath()), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manager.BinaryPath(), []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	var out, diagnostics bytes.Buffer
	if code := runCLI([]string{"--boot-start", paths.Config, paths.Data, paths.Logs}, &out, &diagnostics); code != 1 {
		t.Fatalf("core startup failure hidden: exit=%d %s", code, &diagnostics)
	}
}
