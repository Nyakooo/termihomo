package core

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/Nyakooo/termihomo/internal/platform"
)

// Version is the release verified in docs/TECHNICAL_RESEARCH.md.
const Version = "v1.19.34"

type Manager struct {
	Paths platform.Paths
}

func (m Manager) binary() string {
	return filepath.Join(m.Paths.Data, "cores", Version, "mihomo")
}

func (m Manager) Install(ctx context.Context) error {
	if err := m.Paths.Ensure(); err != nil {
		return err
	}
	lock, err := m.Paths.LockInstance()
	if err != nil {
		return err
	}
	defer lock.Close()
	if _, err := os.Stat(m.binary()); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	var arch, digest string
	switch runtime.GOARCH {
	case "amd64":
		arch = "amd64-v1"
		digest = "0c07340a01408d033dda4088ea2a4c1e4d82aeed7b2f84f7c046e93b6ff3290c"
	case "arm64":
		arch = "arm64"
		digest = "daaf78a6240a282cdfd75bcbf98fe321f6bd52765cc29e79f344191198f0b50f"
	default:
		return errors.New("no Mihomo release for this architecture")
	}
	asset := fmt.Sprintf("https://github.com/MetaCubeX/mihomo/releases/download/%s/mihomo-linux-%s-%s.gz", Version, arch, Version)
	client := &http.Client{Timeout: 2 * time.Minute}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("could not download Mihomo from the official release")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download Mihomo: HTTP %d", resp.StatusCode)
	}

	dir := filepath.Dir(m.binary())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	archive, err := os.CreateTemp(dir, ".mihomo-*.gz")
	if err != nil {
		return err
	}
	defer os.Remove(archive.Name())
	defer archive.Close()
	const archiveLimit = 64 << 20
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(archive, hash), io.LimitReader(resp.Body, archiveLimit+1))
	if err != nil {
		return err
	}
	if size > archiveLimit || hex.EncodeToString(hash.Sum(nil)) != digest {
		return errors.New("Mihomo archive size or SHA-256 verification failed")
	}
	if _, err := archive.Seek(0, io.SeekStart); err != nil {
		return err
	}
	payload, err := gzip.NewReader(archive)
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
		return errors.New("unexpected Mihomo binary size")
	}
	if err := bin.Chmod(0o700); err != nil {
		return err
	}
	if err := bin.Close(); err != nil {
		return err
	}
	return os.Rename(bin.Name(), m.binary())
}
