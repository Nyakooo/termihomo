package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nyakooo/termihomo/internal/core"
	"github.com/Nyakooo/termihomo/internal/i18n"
	"github.com/Nyakooo/termihomo/internal/platform"
)

func TestCommandProgressOnlyForSlowOperations(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want bool
	}{
		{[]string{"install"}, true}, {[]string{"start"}, true}, {[]string{"restart"}, true},
		{[]string{"reload"}, true}, {[]string{"stop"}, true}, {[]string{"down"}, true},
		{[]string{"config", "check"}, true}, {[]string{"profile", "import", "private.yaml"}, true},
		{[]string{"profile", "add", "https://secret.example/subscription"}, true}, {[]string{"profile", "update"}, true},
		{[]string{"proxy", "test", "private-node"}, true},
		{[]string{"status", "--json"}, false}, {[]string{"tun", "status", "--json"}, false},
		{[]string{"sysproxy", "status", "--json"}, false}, {[]string{"mode", "status"}, false},
		{[]string{"profile", "list"}, false}, {[]string{"proxy", "list"}, false},
		{[]string{"proxy", "select", "group", "node"}, false}, {[]string{"config", "backup"}, false},
		{[]string{"rules"}, false}, {[]string{"logs"}, false},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			var out bytes.Buffer
			_, finish := commandProgress(t.Context(), tc.args, &out, i18n.Language("en"))
			if (out.Len() > 0) != tc.want {
				t.Fatalf("unexpected initial feedback: %q", out.String())
			}
			finish()
			if !tc.want && out.Len() != 0 {
				t.Fatalf("query output became noisy: %q", out.String())
			}
			if strings.Contains(out.String(), "secret.example") || strings.Contains(out.String(), "private-node") {
				t.Fatal("progress exposed command arguments")
			}
		})
	}
}

func TestCommandFeedbackDoesNotAlterResults(t *testing.T) {
	isolateCLI(t)
	var out, diagnostics bytes.Buffer
	if code := runCLI([]string{"stop"}, &out, &diagnostics); code != 0 {
		t.Fatal(diagnostics.String())
	}
	if out.Len() != 0 || !strings.Contains(diagnostics.String(), "Done") {
		t.Fatalf("stop feedback: stdout=%q stderr=%q", out.String(), diagnostics.String())
	}
	out.Reset()
	diagnostics.Reset()
	if code := runCLI([]string{"status", "--json"}, &out, &diagnostics); code != 0 {
		t.Fatal(diagnostics.String())
	}
	if !json.Valid(out.Bytes()) || diagnostics.Len() != 0 {
		t.Fatalf("query output polluted: stdout=%q stderr=%q", out.String(), diagnostics.String())
	}
	out.Reset()
	diagnostics.Reset()
	if code := runCLI([]string{"proxy", "test", "missing"}, &out, &diagnostics); code != 1 {
		t.Fatalf("expected failed test, exit=%d", code)
	}
	if !strings.Contains(diagnostics.String(), "Testing proxy") || strings.Contains(diagnostics.String(), "Done") || out.Len() != 0 {
		t.Fatalf("failed operation claimed completion: stdout=%q stderr=%q", out.String(), diagnostics.String())
	}
}

func TestInstallForwardsCoreStagesToStderr(t *testing.T) {
	isolateCLI(t)
	paths, err := platform.ResolvePaths()
	if err != nil {
		t.Fatal(err)
	}
	binary := (core.Manager{Paths: paths}).BinaryPath()
	if err := os.MkdirAll(filepath.Dir(binary), 0700); err != nil {
		t.Fatal(err)
	}
	// An installed core avoids network access; missing bundled databases still
	// exercise the actual Core Manager progress callback through the CLI.
	if err := os.WriteFile(binary, nil, 0700); err != nil {
		t.Fatal(err)
	}
	var out, diagnostics bytes.Buffer
	if code := runCLI([]string{"install"}, &out, &diagnostics); code != 0 {
		t.Fatal(diagnostics.String())
	}
	if !strings.Contains(diagnostics.String(), "Preparing local databases") || !strings.Contains(diagnostics.String(), "Done") {
		t.Fatalf("core stages missing: %q", diagnostics.String())
	}
	if strings.Contains(out.String(), "Preparing") || !strings.Contains(out.String(), "installed") {
		t.Fatalf("stdout result changed: %q", out.String())
	}
}
