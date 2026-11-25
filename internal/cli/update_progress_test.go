package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Nyakooo/termihomo/internal/i18n"
	"github.com/Nyakooo/termihomo/internal/update"
)

func TestUpdateProgressOutput(t *testing.T) {
	for _, interactive := range []bool{false, true} {
		for _, total := range []int64{-1, 2 << 20} {
			var out bytes.Buffer
			report, finish := updateProgress(&out, i18n.Language("en"), interactive)
			report(update.Progress{Stage: "Checking for updates"})
			if !strings.Contains(out.String(), "Checking for updates") {
				t.Fatal("initial request has no visible feedback")
			}
			report(update.Progress{Stage: "Downloading update"})
			report(update.Progress{Stage: "Downloading update", Downloaded: 1 << 20, Total: total})
			if interactive {
				if !strings.Contains(out.String(), "1.0") {
					t.Fatal("download size missing")
				}
				if total > 0 && !strings.Contains(out.String(), "50%") {
					t.Fatal("known-size download has no percentage")
				}
				if total < 0 && strings.Contains(out.String(), "%") {
					t.Fatal("unknown-size download fabricated percentage")
				}
			}
			report(update.Progress{Stage: "Downloading update", Downloaded: 2 << 20, Total: total})
			if interactive && total > 0 && !strings.Contains(out.String(), "100%") {
				t.Fatal("completion was throttled")
			}
			report(update.Progress{Stage: "Verifying download"})
			report(update.Progress{Stage: "Installing update"})
			finish()
			if !interactive && (strings.Contains(out.String(), "\r") || strings.Contains(out.String(), "MiB")) {
				t.Fatal("redirected output contains live progress noise")
			}
			if strings.Count(out.String(), "Downloading update…") != 1 {
				t.Fatal("duplicate download stage lines")
			}
			if !strings.HasSuffix(out.String(), "\n") {
				t.Fatal("progress left output on an unfinished line")
			}
		}
	}
}

func TestUpdateProgressFailureFinishesLine(t *testing.T) {
	var out bytes.Buffer
	report, finish := updateProgress(&out, i18n.Language("zh-CN"), true)
	report(update.Progress{Stage: "Downloading update", Downloaded: 1 << 20, Total: -1})
	finish()
	if !strings.Contains(out.String(), "正在下载更新") || !strings.HasSuffix(out.String(), "\n") {
		t.Fatalf("failed download output is incomplete: %q", out.String())
	}
}
