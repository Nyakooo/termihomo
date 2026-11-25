package core

import (
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Nyakooo/termihomo/internal/i18n"
	"github.com/Nyakooo/termihomo/internal/platform"
)

// Version is the supported Mihomo release.
const Version = "v1.19.34"

type Manager struct {
	Paths            platform.Paths
	proxyOrder       *proxyOrderCache
	profileSummaries *profileSummaryCache
}

func (m Manager) binary() string {
	name := "mihomo"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(m.Paths.Data, "cores", Version, name)
}

// BinaryPath returns the managed Mihomo binary location for display purposes.
func (m Manager) BinaryPath() string { return m.binary() }

// KernelPresent reports whether the managed Mihomo binary is already installed.
func (m Manager) KernelPresent() bool {
	_, err := os.Stat(m.binary())
	return err == nil
}

func (m Manager) Install(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := m.Paths.Ensure(); err != nil {
		return err
	}
	lock, err := m.Paths.LockInstance()
	if err != nil {
		return err
	}
	defer lock.Close()
	if _, err := os.Stat(m.binary()); err == nil {
		return m.ensureGeodata(ctx)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	reportProfileProgress(ctx, "Preparing Mihomo")
	asset, digest := kernelAsset(runtime.GOOS, runtime.GOARCH)
	if asset == "" {
		return i18n.Message("no Mihomo release for this architecture")
	}
	official := fmt.Sprintf("https://github.com/MetaCubeX/mihomo/releases/download/%s/%s", Version, asset)
	sources := []string{official, "https://gh-proxy.com/" + official, "https://ghproxy.net/" + official}
	dir := filepath.Dir(m.binary())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	reportProfileProgress(ctx, "Downloading Mihomo")
	archive, size, err := downloadKernelArchive(ctx, &http.Client{Timeout: 2 * time.Minute}, sources, dir, digest)
	if err != nil {
		return err
	}
	defer os.Remove(archive.Name())
	defer archive.Close()
	reportProfileProgress(ctx, "Extracting Mihomo")
	payload, err := kernelPayload(archive, size, filepath.Ext(asset) == ".zip")
	if err != nil {
		return err
	}
	defer payload.Close()
	bin, err := os.CreateTemp(dir, ".mihomo-*")
	if err != nil {
		return err
	}
	defer os.Remove(bin.Name())
	defer bin.Close()
	const binaryLimit = 160 << 20
	size, err = io.Copy(bin, io.LimitReader(payload, binaryLimit+1))
	if err != nil {
		return err
	}
	if size == 0 || size > binaryLimit {
		return i18n.Message("unexpected Mihomo binary size")
	}
	if err := bin.Chmod(0o700); err != nil {
		return err
	}
	if err := bin.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(bin.Name(), m.binary()); err != nil {
		return err
	}
	return m.ensureGeodata(ctx)
}

// Race complete, verified downloads so a fast response with an invalid or slow
// body cannot prevent another source from winning.
func downloadKernelArchive(ctx context.Context, client *http.Client, sources []string, dir, digest string) (*os.File, int64, error) {
	downloadCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	type result struct {
		archive *os.File
		size    int64
		err     error
	}
	results := make(chan result)
	for _, source := range sources {
		go func() {
			archive, size, err := fetchKernelArchive(downloadCtx, client, source, dir, digest)
			results <- result{archive, size, err}
		}()
	}

	var winner result
	var failures []string
	for range sources {
		attempt := <-results
		if attempt.err != nil {
			failures = append(failures, attempt.err.Error())
		} else if winner.archive == nil {
			winner = attempt
			cancel()
		} else {
			attempt.archive.Close()
			os.Remove(attempt.archive.Name())
		}
	}
	if err := ctx.Err(); err != nil {
		if winner.archive != nil {
			winner.archive.Close()
			os.Remove(winner.archive.Name())
		}
		return nil, 0, err
	}
	if winner.archive == nil {
		return nil, 0, i18n.Errorf("could not download Mihomo from any source: %s", strings.Join(failures, " | "))
	}
	return winner.archive, winner.size, nil
}

func fetchKernelArchive(ctx context.Context, client *http.Client, source, dir, digest string) (*os.File, int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		var requestErr *url.Error
		if errors.As(err, &requestErr) {
			err = requestErr.Err
		}
		return nil, 0, fmt.Errorf("%s: %w", req.URL.Host, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("%s: HTTP %d", req.URL.Host, resp.StatusCode)
	}

	archive, err := os.CreateTemp(dir, ".mihomo-archive-*")
	if err != nil {
		return nil, 0, err
	}
	verified := false
	defer func() {
		if !verified {
			archive.Close()
			os.Remove(archive.Name())
		}
	}()
	const archiveLimit = 64 << 20
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(archive, hash), io.LimitReader(resp.Body, archiveLimit+1))
	if err != nil {
		return nil, 0, fmt.Errorf("%s: %w", req.URL.Host, err)
	}
	if size > archiveLimit || hex.EncodeToString(hash.Sum(nil)) != digest {
		return nil, 0, fmt.Errorf("%s: %w", req.URL.Host, i18n.Message("Mihomo archive size or SHA-256 verification failed"))
	}
	verified = true
	return archive, size, nil
}

func kernelAsset(goos, goarch string) (string, string) {
	arch, extension := goarch, "gz"
	if goarch == "amd64" {
		arch = "amd64-v1"
	}
	var digest string
	switch goos + "/" + goarch {
	case "linux/amd64":
		digest = "0c07340a01408d033dda4088ea2a4c1e4d82aeed7b2f84f7c046e93b6ff3290c"
	case "linux/arm64":
		digest = "daaf78a6240a282cdfd75bcbf98fe321f6bd52765cc29e79f344191198f0b50f"
	case "android/arm64":
		arch, digest = "arm64-v8", "48a71fc6a68e2909587176e0ab7185d8f2fac298fa96ccdacf30ea86ab2359b5"
	case "darwin/amd64":
		digest = "84af3a9ffd4699e02f14b89859c58b4fc380f6a2135037c6e88e6822347ae657"
	case "darwin/arm64":
		digest = "1a7a20eca63fbbaec90137717465bfde488d0db2a3a79b8fadebf9b2545b6e4c"
	case "windows/amd64":
		extension, digest = "zip", "703e325bfeee582f874b06d054eaae55ab7c194645d1835411685aafa85da5ea"
	case "windows/arm64":
		extension, digest = "zip", "076cb04a21377675bcf009b8d1dd83006ce2464aa537e37bce1faf6d3504f554"
	default:
		return "", ""
	}
	return fmt.Sprintf("mihomo-%s-%s-%s.%s", goos, arch, Version, extension), digest
}

func kernelPayload(archive *os.File, size int64, zipped bool) (io.ReadCloser, error) {
	if !zipped {
		if _, err := archive.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
		return gzip.NewReader(archive)
	}
	files, err := zip.NewReader(archive, size)
	if err != nil {
		return nil, err
	}
	for _, file := range files.File {
		if file.Mode().IsRegular() && strings.EqualFold(filepath.Ext(file.Name), ".exe") {
			return file.Open()
		}
	}
	return nil, i18n.Message("Mihomo archive has no Windows executable")
}
