package update

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Nyakooo/termihomo/internal/i18n"
	"github.com/Nyakooo/termihomo/internal/platform"
)

const (
	repository = "Nyakooo/termihomo"
	githubAPI  = "https://api.github.com/repos/" + repository
	maxBinary  = 64 << 20
)

// ErrUpdateScheduled means Windows will replace the executable after this process exits.
var ErrUpdateScheduled = errors.New("executable replacement scheduled")

type release struct {
	Tag    string `json:"tag_name"`
	Assets []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"assets"`
}

func Run(ctx context.Context, current string) (string, bool, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", false, err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return "", false, err
	}
	token := os.Getenv("GH_TOKEN")
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	return fetch(ctx, &http.Client{Timeout: 2 * time.Minute}, githubAPI, token,
		current, executable, runtime.GOOS, runtime.GOARCH)
}

func releaseAsset(goos, goarch string) (asset, member string, err error) {
	if (goarch != "amd64" && goarch != "arm64") ||
		(goos != "linux" && goos != "darwin" && goos != "windows" && !(goos == "android" && goarch == "arm64")) {
		return "", "", i18n.Message("No Termihomo release for this platform")
	}
	name := "termihomo-" + goos + "-" + goarch
	switch goos {
	case "windows":
		return name + ".exe", "", nil
	case "android":
		return name + ".tar.gz", name, nil
	default:
		return name + ".tar.gz", "termihomo", nil
	}
}

func stableVersion(tag string) ([3]uint64, bool) {
	var version [3]uint64
	if !strings.HasPrefix(tag, "v") {
		return version, false
	}
	parts := strings.Split(tag[1:], ".")
	if len(parts) != len(version) {
		return version, false
	}
	for i, part := range parts {
		n, err := strconv.ParseUint(part, 10, 64)
		if err != nil || strconv.FormatUint(n, 10) != part {
			return version, false
		}
		version[i] = n
	}
	return version, true
}

func wantsUpdate(current, latest string) (bool, error) {
	next, ok := stableVersion(latest)
	if !ok {
		return false, i18n.Message("The latest Termihomo release has an invalid version")
	}
	if current == "dev" {
		return true, nil
	}
	installed, ok := stableVersion(current)
	if !ok {
		return false, i18n.Message("The installed Termihomo version cannot be compared")
	}
	for i, part := range next {
		if part != installed[i] {
			return part > installed[i], nil
		}
	}
	return false, nil
}

func fetch(ctx context.Context, client *http.Client, api, token, current, executable, goos, goarch string) (string, bool, error) {
	asset, member, err := releaseAsset(goos, goarch)
	if err != nil {
		return "", false, err
	}
	// Stat the open file so Windows records its identity before any replacement.
	runningFile, err := os.Open(executable)
	if err != nil {
		return "", false, err
	}
	original, err := runningFile.Stat()
	runningFile.Close()
	if err != nil {
		return "", false, err
	}
	get := func(endpoint, accept string) (io.ReadCloser, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", accept)
		req.Header.Set("User-Agent", "Termihomo/"+current)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, i18n.Message("Could not download the Termihomo release")
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			if resp.StatusCode == http.StatusNotFound {
				return nil, i18n.Message("Termihomo release not found. For a private repository set GH_TOKEN with read access.")
			}
			return nil, i18n.Errorf("Termihomo release request returned HTTP %d", resp.StatusCode)
		}
		return resp.Body, nil
	}

	body, err := get(api+"/releases/latest", "application/vnd.github+json")
	if err != nil {
		return "", false, err
	}
	var latest release
	err = json.NewDecoder(io.LimitReader(body, 2<<20)).Decode(&latest)
	body.Close()
	if err != nil {
		return "", false, err
	}
	update, err := wantsUpdate(current, latest.Tag)
	if err != nil || !update {
		return current, false, err
	}

	assets := make(map[string]int64)
	for _, file := range latest.Assets {
		if _, exists := assets[file.Name]; exists {
			return "", false, i18n.Message("The Termihomo release contains duplicate assets")
		}
		assets[file.Name] = file.ID
	}
	if assets[asset] <= 0 || assets["SHA256SUMS"] <= 0 {
		return "", false, i18n.Message("The Termihomo release is missing the platform asset or SHA256SUMS")
	}
	download := func(name string) (io.ReadCloser, error) {
		if token == "" && api == githubAPI {
			return get("https://github.com/"+repository+"/releases/download/"+latest.Tag+"/"+name, "application/octet-stream")
		}
		return get(fmt.Sprintf("%s/releases/assets/%d", api, assets[name]), "application/octet-stream")
	}
	sums, err := download("SHA256SUMS")
	if err != nil {
		return "", false, err
	}
	const maxManifest = 64 << 10
	manifest, err := io.ReadAll(io.LimitReader(sums, maxManifest+1))
	sums.Close()
	if err != nil {
		return "", false, err
	}
	if len(manifest) > maxManifest {
		return "", false, i18n.Message("Invalid Termihomo checksum manifest")
	}
	expected, err := checksum(manifest, asset)
	if err != nil {
		return "", false, err
	}

	payload, err := download(asset)
	if err != nil {
		return "", false, err
	}
	defer payload.Close()
	file, err := os.CreateTemp(filepath.Dir(executable), ".termihomo-download-*")
	if err != nil {
		return "", false, err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(payload, maxBinary+1))
	if err != nil {
		return "", false, err
	}
	if n == 0 || n > maxBinary || hex.EncodeToString(hash.Sum(nil)) != expected {
		return "", false, i18n.Message("Termihomo SHA-256 verification failed")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", false, err
	}
	lock, err := platform.LockExecutableUpdate(executable)
	if err != nil {
		return "", false, err
	}
	defer lock.Close()
	if err := install(executable, file, member, original); err != nil {
		if errors.Is(err, ErrUpdateScheduled) {
			return latest.Tag, false, err
		}
		return "", false, err
	}
	return latest.Tag, true, nil
}

func checksum(data []byte, asset string) (string, error) {
	var result string
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != asset {
			continue
		}
		digest, err := hex.DecodeString(fields[0])
		if err != nil || len(digest) != sha256.Size || result != "" {
			return "", i18n.Message("Invalid Termihomo checksum manifest")
		}
		result = strings.ToLower(fields[0])
	}
	if result == "" {
		return "", i18n.Message("Invalid Termihomo checksum manifest")
	}
	return result, nil
}

func install(executable string, payload io.Reader, member string, original os.FileInfo) error {
	if member != "" {
		reader, err := gzip.NewReader(payload)
		if err != nil {
			return err
		}
		defer reader.Close()
		archive := tar.NewReader(reader)
		header, err := archive.Next()
		if err != nil {
			return err
		}
		if header.Name != member || header.Typeflag != tar.TypeReg || header.Size <= 0 || header.Size > maxBinary {
			return i18n.Message("Unexpected Termihomo archive contents")
		}
		payload = archive
	}
	info, err := os.Stat(executable)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return i18n.Message("The Termihomo executable is not a regular file")
	}
	if !os.SameFile(original, info) {
		return i18n.Message("The Termihomo executable changed during the update. Run the command again.")
	}
	file, err := os.CreateTemp(filepath.Dir(executable), ".termihomo-update-*")
	if err != nil {
		return err
	}
	var scheduled bool
	defer func() {
		if !scheduled {
			os.Remove(file.Name())
		}
	}()
	defer file.Close()
	n, err := io.Copy(file, io.LimitReader(payload, maxBinary+1))
	if err != nil {
		return err
	}
	if n == 0 || n > maxBinary {
		return i18n.Message("Invalid Termihomo executable size")
	}
	if err := file.Chmod(info.Mode().Perm()); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	err = replaceExecutable(file.Name(), executable)
	scheduled = errors.Is(err, ErrUpdateScheduled)
	return err
}
