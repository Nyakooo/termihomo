package core

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nyakooo/termihomo/internal/platform"
)

func writeManifest(t *testing.T, m Manager, manifest map[string]int64) {
	t.Helper()
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.geodataManifestPath(), body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureGeodataSkipsCompleteCache(t *testing.T) {
	m := Manager{Paths: platform.Paths{Data: t.TempDir()}}
	if err := os.MkdirAll(m.runDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := make(map[string]int64)
	for _, file := range geodataFiles {
		body := []byte("cached-" + file.name)
		if err := os.WriteFile(filepath.Join(m.runDir(), file.name), body, 0o600); err != nil {
			t.Fatal(err)
		}
		manifest[file.name] = int64(len(body))
	}
	writeManifest(t, m, manifest)
	if err := m.ensureGeodata(context.Background()); err != nil {
		t.Fatalf("ensureGeodata with a complete cache: %v", err)
	}
	for _, file := range geodataFiles {
		body, err := os.ReadFile(filepath.Join(m.runDir(), file.name))
		if err != nil || string(body) != "cached-"+file.name {
			t.Fatalf("cached %s was modified: %q err=%v", file.name, body, err)
		}
	}
}

func TestEnsureGeodataRedownloadsPartial(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "complete-payload")
	}))
	defer server.Close()
	orig := geodataFiles
	defer func() { geodataFiles = orig }()
	geodataFiles = []struct {
		name string
		url  string
	}{{"GeoSite.dat", server.URL}}

	m := Manager{Paths: platform.Paths{Data: t.TempDir()}}
	if err := os.MkdirAll(m.runDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(m.runDir(), "GeoSite.dat")
	if err := os.WriteFile(target, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeManifest(t, m, map[string]int64{"GeoSite.dat": 999})
	if err := m.ensureGeodata(context.Background()); err != nil {
		t.Fatalf("ensureGeodata: %v", err)
	}
	body, err := os.ReadFile(target)
	if err != nil || string(body) != "complete-payload" {
		t.Fatalf("partial file should be replaced, got %q err=%v", body, err)
	}
	if m.readGeodataManifest()["GeoSite.dat"] != int64(len("complete-payload")) {
		t.Fatal("manifest should record the complete size")
	}
}

func TestDownloadGeodata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "geodata-payload")
	}))
	defer server.Close()
	m := Manager{Paths: platform.Paths{Data: t.TempDir()}}
	if err := os.MkdirAll(m.runDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(m.runDir(), "GeoSite.dat")
	size, err := m.downloadGeodata(context.Background(), server.URL, target)
	if err != nil {
		t.Fatalf("downloadGeodata: %v", err)
	}
	body, err := os.ReadFile(target)
	if err != nil || string(body) != "geodata-payload" || size != int64(len(body)) {
		t.Fatalf("downloaded content=%q size=%d err=%v", body, size, err)
	}
}

func TestDownloadGeodataReportsFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	m := Manager{Paths: platform.Paths{Data: t.TempDir()}}
	if err := os.MkdirAll(m.runDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(m.runDir(), "GeoSite.dat")
	if _, err := m.downloadGeodata(context.Background(), server.URL, target); err == nil {
		t.Fatal("expected an error for HTTP 500")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("a failed download must not leave a partial file")
	}
}

func TestValidationDetailRedactsSecretAndURL(t *testing.T) {
	body := []byte("secret: supersecretvalue\nmixed-port: 7890\n")
	output := strings.Join([]string{
		`time="x" level=info msg="Start initial configuration in progress"`,
		`time="x" level=error msg="proxy 0: ss cipher bad initialize error: unknown method, secret supersecretvalue"`,
		`time="x" level=error msg="provider sub https://secret.example/sub?token=abc failed"`,
		`configuration file /tmp/.config-XYZ.yaml test failed`,
	}, "\n")
	detail := validationDetail(output, body, "/tmp/.config-XYZ.yaml")
	if !strings.Contains(detail, "unknown method") {
		t.Fatalf("detail should keep the real reason, got %q", detail)
	}
	if strings.Contains(detail, "supersecretvalue") {
		t.Fatalf("detail leaked the API secret: %q", detail)
	}
	if strings.Contains(detail, "https://secret.example") || strings.Contains(detail, "token=abc") {
		t.Fatalf("detail leaked a URL: %q", detail)
	}
	if !strings.Contains(detail, "[url]") {
		t.Fatalf("detail should mark redacted URLs: %q", detail)
	}
	if strings.Contains(detail, ".config-XYZ") {
		t.Fatalf("detail leaked the temp config path: %q", detail)
	}
	if strings.Contains(detail, "Start initial configuration") {
		t.Fatalf("detail should drop info noise: %q", detail)
	}
}
