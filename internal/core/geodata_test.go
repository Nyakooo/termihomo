package core

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nyakooo/termihomo/internal/platform"
)

func writeManifest(t *testing.T, m Manager, manifest map[string]int64) {
	t.Helper()
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.geodataManifestPath(), body, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureGeodataExtractsOfflineAndRepairsPartial(t *testing.T) {
	m := Manager{Paths: platform.Paths{Data: t.TempDir()}}
	if err := m.ensureGeodata(t.Context()); err != nil {
		t.Fatal(err)
	}
	files, err := geodataFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 4 {
		t.Fatalf("want all four database formats, got %d", len(files))
	}
	for _, file := range files {
		path := filepath.Join(m.runDir(), file.Name)
		body, err := os.ReadFile(path)
		if err != nil || int64(len(body)) != file.Size {
			t.Fatalf("%s: size=%d err=%v", file.Name, len(body), err)
		}
		if err := os.WriteFile(path, []byte("partial"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.ensureGeodata(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		info, err := os.Stat(filepath.Join(m.runDir(), file.Name))
		if err != nil || info.Size() != file.Size || m.readGeodataManifest()[file.Name] != file.Size {
			t.Fatalf("%s was not repaired: %v", file.Name, err)
		}
	}
}

func TestEnsureGeodataSkipsCompleteCache(t *testing.T) {
	m := Manager{Paths: platform.Paths{Data: t.TempDir()}}
	if err := os.MkdirAll(m.runDir(), 0700); err != nil {
		t.Fatal(err)
	}
	files, err := geodataFiles()
	if err != nil {
		t.Fatal(err)
	}
	manifest := make(map[string]int64)
	for _, file := range files {
		body := []byte("cached-" + file.Name)
		if err := os.WriteFile(filepath.Join(m.runDir(), file.Name), body, 0600); err != nil {
			t.Fatal(err)
		}
		manifest[file.Name] = int64(len(body))
	}
	writeManifest(t, m, manifest)
	if err := m.ensureGeodata(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		body, err := os.ReadFile(filepath.Join(m.runDir(), file.Name))
		if err != nil || string(body) != "cached-"+file.Name {
			t.Fatalf("cached database changed: %s, %v", file.Name, err)
		}
	}
}

func TestEnsureGeodataCancellationDoesNotWriteFiles(t *testing.T) {
	m := Manager{Paths: platform.Paths{Data: t.TempDir()}}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := m.ensureGeodata(ctx); err != context.Canceled {
		t.Fatalf("got %v", err)
	}
	if _, err := os.Stat(m.geodataManifestPath()); !os.IsNotExist(err) {
		t.Fatal("cancelled extraction wrote manifest")
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

func TestEnsureGeodataPreservesUpdatedAndUntrackedDatabases(t *testing.T) {
	m := Manager{Paths: platform.Paths{Data: t.TempDir()}}
	if err := m.ensureGeodata(t.Context()); err != nil {
		t.Fatal(err)
	}
	files, err := geodataFiles()
	if err != nil {
		t.Fatal(err)
	}
	// A complete extra protobuf entry models a valid core-managed DAT update.
	path := filepath.Join(m.runDir(), "GeoSite.dat")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, 0x0a, 0x04, 0x0a, 0x02, 'z', 'z')
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.ensureGeodata(t.Context()); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(saved, body) {
		t.Fatal("valid updated DAT was overwritten")
	}
	if m.readGeodataManifest()["GeoSite.dat"] != int64(len(body)) {
		t.Fatal("updated size not adopted")
	}
	// Missing/null metadata must adopt complete MMDB and DAT files, not replace
	// user snapshots. Their modification times reveal any unnecessary rewrite.
	stamp := time.Unix(1000000000, 0)
	for _, file := range files {
		if err := os.Chtimes(filepath.Join(m.runDir(), file.Name), stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(m.geodataManifestPath(), []byte("null"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.ensureGeodata(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		info, err := os.Stat(filepath.Join(m.runDir(), file.Name))
		if err != nil || !info.ModTime().Equal(stamp) {
			t.Fatalf("valid %s was rewritten: %v", file.Name, err)
		}
	}
}

func TestGeodataRejectsTruncatedFrames(t *testing.T) {
	valid := []byte{0x0a, 0x04, 0x0a, 0x02, 'c', 'n'}
	if !validGeodataList(valid) {
		t.Fatal("valid protobuf rejected")
	}
	for n := 0; n < len(valid); n++ {
		if validGeodataList(valid[:n]) {
			t.Fatalf("accepted prefix of length %d", n)
		}
	}
}
