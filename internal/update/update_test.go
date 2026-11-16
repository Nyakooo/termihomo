package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFetchVerifiedRelease(t *testing.T) {
	for _, tc := range []struct {
		name, current, member, expected string
		corrupt, changed, fails         bool
	}{
		{name: "install newer", current: "v0.1.9", member: "termihomo", expected: "new", changed: true},
		{name: "no downgrade", current: "v0.11.0", member: "termihomo", expected: "old"},
		{name: "reject corrupt download", current: "v0.1.9", member: "termihomo", expected: "old", corrupt: true, fails: true},
		{name: "reject unexpected archive path", current: "v0.1.9", member: "../termihomo", expected: "old", fails: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var archive bytes.Buffer
			gzipWriter := gzip.NewWriter(&archive)
			writer := tar.NewWriter(gzipWriter)
			if err := writer.WriteHeader(&tar.Header{Name: tc.member, Mode: 0o755, Size: 3}); err != nil {
				t.Fatal(err)
			}
			if _, err := writer.Write([]byte("new")); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			if err := gzipWriter.Close(); err != nil {
				t.Fatal(err)
			}

			digest := sha256.Sum256(archive.Bytes())
			if tc.corrupt {
				digest[0] ^= 0xff
			}
			asset := "termihomo-linux-amd64.tar.gz"
			mux := http.NewServeMux()
			mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer test-token" {
					http.Error(w, "missing token", http.StatusUnauthorized)
					return
				}
				fmt.Fprintf(w, `{"tag_name":"v0.10.0","assets":[{"id":1,"name":%q},{"id":2,"name":"SHA256SUMS"}]}`, asset)
			})
			mux.HandleFunc("/releases/assets/1", func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Accept") != "application/octet-stream" {
					http.Error(w, "incorrect accept header", http.StatusBadRequest)
					return
				}
				_, _ = w.Write(archive.Bytes())
			})
			mux.HandleFunc("/releases/assets/2", func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintf(w, "%x  %s\n", digest, asset)
			})
			server := httptest.NewServer(mux)
			defer server.Close()

			path := filepath.Join(t.TempDir(), "termihomo")
			if err := os.WriteFile(path, []byte("old"), 0o755); err != nil {
				t.Fatal(err)
			}
			tag, changed, err := fetch(t.Context(), server.Client(), server.URL, "test-token", tc.current, path, "linux", "amd64")
			if (err != nil) != tc.fails || changed != tc.changed {
				t.Fatalf("fetch returned tag=%q changed=%v err=%v", tag, changed, err)
			}
			body, err := os.ReadFile(path)
			if err != nil || string(body) != tc.expected {
				t.Fatalf("executable contents=%q err=%v", body, err)
			}
		})
	}
}

func TestReleaseAssetNames(t *testing.T) {
	for _, tc := range []struct{ goos, arch, asset, member string }{
		{"linux", "amd64", "termihomo-linux-amd64.tar.gz", "termihomo"},
		{"darwin", "arm64", "termihomo-darwin-arm64.tar.gz", "termihomo"},
		{"windows", "arm64", "termihomo-windows-arm64.exe", ""},
		{"android", "arm64", "termihomo-android-arm64.tar.gz", "termihomo-android-arm64"},
	} {
		asset, member, err := releaseAsset(tc.goos, tc.arch)
		if err != nil || asset != tc.asset || member != tc.member {
			t.Fatalf("%s/%s: asset=%q member=%q err=%v", tc.goos, tc.arch, asset, member, err)
		}
	}
}

func TestFetchRejectsReplacementDuringDownload(t *testing.T) {
	dir := t.TempDir()
	executable := filepath.Join(dir, "termihomo")
	if err := os.WriteFile(executable, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(dir, "replacement")
	if err := os.WriteFile(replacement, []byte("newest"), 0o755); err != nil {
		t.Fatal(err)
	}
	const asset = "termihomo-windows-amd64.exe"
	payload := []byte("new")
	replaceErr := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases/latest":
			fmt.Fprintf(w, `{"tag_name":"v0.2.0","assets":[{"id":1,"name":%q},{"id":2,"name":"SHA256SUMS"}]}`, asset)
		case "/releases/assets/2":
			fmt.Fprintf(w, "%x  %s\n", sha256.Sum256(payload), asset)
		case "/releases/assets/1":
			replaceErr <- os.Rename(replacement, executable)
			w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	_, changed, err := fetch(t.Context(), server.Client(), server.URL, "", "v0.1.0", executable, "windows", "amd64")
	select {
	case err := <-replaceErr:
		if err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatal("the update never downloaded the executable")
	}
	if err == nil || changed {
		t.Fatalf("overlapping update changed=%v err=%v", changed, err)
	}
	body, err := os.ReadFile(executable)
	if err != nil || string(body) != "newest" {
		t.Fatalf("executable contents=%q err=%v", body, err)
	}
}
