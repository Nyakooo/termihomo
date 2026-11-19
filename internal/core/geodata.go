package core

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Nyakooo/termihomo/internal/i18n"
)

// The release executable carries the same four database formats as FlClash.
// Extraction is local and never blocks profile import on a GitHub download.
//
//go:embed geodata/*
var bundledGeodata embed.FS

type geodataFile struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

func geodataFiles() ([]geodataFile, error) {
	body, err := bundledGeodata.ReadFile("geodata/manifest.json")
	if err != nil {
		return nil, err
	}
	var files []geodataFile
	err = json.Unmarshal(body, &files)
	return files, err
}

func (m Manager) geodataManifestPath() string {
	return filepath.Join(m.Paths.Data, "geodata.json")
}

func (m Manager) readGeodataManifest() map[string]int64 {
	manifest := make(map[string]int64)
	body, err := os.ReadFile(m.geodataManifestPath())
	if err == nil {
		_ = json.Unmarshal(body, &manifest)
	}
	if manifest == nil {
		manifest = make(map[string]int64)
	}
	return manifest
}

// Keep complete cached databases, including previously downloaded versions.
// Missing or known partial files are repaired from the embedded snapshot.
func (m Manager) ensureGeodata(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(m.runDir(), 0700); err != nil {
		return err
	}
	files, err := geodataFiles()
	if err != nil {
		return err
	}
	ready := func(manifest map[string]int64, file geodataFile) bool {
		info, err := os.Stat(filepath.Join(m.runDir(), file.Name))
		return err == nil && info.Mode().IsRegular() && info.Size() > 0 && manifest[file.Name] == info.Size()
	}
	manifest := m.readGeodataManifest()
	complete := true
	for _, file := range files {
		complete = complete && ready(manifest, file)
	}
	if complete {
		return nil
	}
	lock, err := m.Paths.LockGeodata()
	if err != nil {
		return err
	}
	defer lock.Close()
	manifest = m.readGeodataManifest()
	reportProfileProgress(ctx, "Preparing local databases")
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if ready(manifest, file) {
			continue
		}
		target := filepath.Join(m.runDir(), file.Name)
		if validGeodata(target, file.Name) {
			info, err := os.Stat(target)
			if err != nil {
				return err
			}
			manifest[file.Name] = info.Size()
		} else {
			if err := m.extractGeodata(ctx, file); err != nil {
				return err
			}
			manifest[file.Name] = file.Size
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		// Persist each completed file so cancellation does not repeat its extraction.
		body, err := json.Marshal(manifest)
		if err != nil {
			return err
		}
		if err := atomicWrite(m.geodataManifestPath(), body); err != nil {
			return err
		}
	}
	return nil
}

func (m Manager) extractGeodata(ctx context.Context, file geodataFile) error {
	compressed, err := bundledGeodata.Open("geodata/" + file.Name + ".gz")
	if err != nil {
		return err
	}
	defer compressed.Close()
	reader, err := gzip.NewReader(compressed)
	if err != nil {
		return err
	}
	defer reader.Close()
	body, err := io.ReadAll(io.LimitReader(reader, file.Size+1))
	if err != nil {
		return err
	}
	digest := sha256.Sum256(body)
	if int64(len(body)) != file.Size || hex.EncodeToString(digest[:]) != file.SHA256 {
		return fmt.Errorf("%s: %w", file.Name, i18n.Message("Bundled database verification failed"))
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return atomicWrite(filepath.Join(m.runDir(), file.Name), body)
}
