package core

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Nyakooo/termihomo/internal/i18n"
)

// geodataFiles maps the local filenames Mihomo expects to their official
// release URLs. Mihomo's built-in downloader gives up after ~90s, which is too
// short on slow networks, so Termihomo fetches them with a generous timeout and
// caches them in the run directory for validation and startup to read locally.
var geodataFiles = []struct {
	name string
	url  string
}{
	{"geoip.metadb", "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geoip.metadb"},
	{"GeoSite.dat", "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geosite.dat"},
}

func (m Manager) geodataManifestPath() string {
	return filepath.Join(m.Paths.Data, "geodata.json")
}

func (m Manager) readGeodataManifest() map[string]int64 {
	manifest := make(map[string]int64)
	body, err := os.ReadFile(m.geodataManifestPath())
	if err != nil {
		return manifest
	}
	_ = json.Unmarshal(body, &manifest)
	return manifest
}

// ensureGeodata makes sure complete GeoIP/GeoSite databases are present before
// Mihomo validates or starts. A file is trusted only when its size matches the
// recorded complete download; this rejects partial files left behind by an
// interrupted Mihomo download, which Mihomo would otherwise try to re-fetch.
func (m Manager) ensureGeodata(ctx context.Context) error {
	if err := os.MkdirAll(m.runDir(), 0o700); err != nil {
		return err
	}
	manifest := m.readGeodataManifest()
	if m.geodataReady(manifest) {
		return nil
	}
	lock, err := m.Paths.LockGeodata()
	if err != nil {
		return err
	}
	defer lock.Close()
	manifest = m.readGeodataManifest()
	changed := false
	for _, file := range geodataFiles {
		target := filepath.Join(m.runDir(), file.name)
		if info, err := os.Stat(target); err == nil && info.Size() > 0 && manifest[file.name] == info.Size() {
			continue
		}
		size, err := m.downloadGeodata(ctx, file.url, target)
		if err != nil {
			return err
		}
		manifest[file.name] = size
		changed = true
	}
	if !changed {
		return nil
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	return atomicWrite(m.geodataManifestPath(), body)
}

func (m Manager) geodataReady(manifest map[string]int64) bool {
	for _, file := range geodataFiles {
		info, err := os.Stat(filepath.Join(m.runDir(), file.name))
		if err != nil || info.Size() <= 0 || manifest[file.name] != info.Size() {
			return false
		}
	}
	return true
}

func (m Manager) downloadGeodata(ctx context.Context, source, target string) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return 0, err
	}
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return 0, i18n.Message("could not download Mihomo GeoIP/GeoSite data")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, i18n.Errorf("download GeoIP/GeoSite data: HTTP %d", resp.StatusCode)
	}
	tmp, err := os.CreateTemp(m.runDir(), ".geodata-*")
	if err != nil {
		return 0, err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	const limit = 64 << 20
	size, err := io.Copy(tmp, io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return 0, i18n.Message("could not download Mihomo GeoIP/GeoSite data")
	}
	if size == 0 || size > limit {
		return 0, i18n.Message("unexpected GeoIP/GeoSite data size")
	}
	if err := tmp.Close(); err != nil {
		return 0, err
	}
	if err := os.Rename(tmp.Name(), target); err != nil {
		return 0, err
	}
	return size, nil
}
