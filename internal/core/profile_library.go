package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Nyakooo/termihomo/internal/i18n"
)

func (m Manager) libraryDir() string {
	return filepath.Join(m.Paths.Config, "profiles")
}

func (m Manager) libraryIDs() ([]string, error) {
	entries, err := os.ReadDir(m.libraryDir())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, entry := range entries {
		id := entry.Name()
		if len(id) == 32 {
			if _, err := hex.DecodeString(id); err == nil {
				ids = append(ids, id)
			}
		}
	}
	return ids, nil
}

func (m Manager) libraryProfiles() ([]profile, error) {
	ids, err := m.libraryIDs()
	if err != nil {
		return nil, err
	}
	records := make([]profile, 0, len(ids))
	for _, id := range ids {
		item, err := readProfileAt(filepath.Join(m.libraryDir(), id))
		if err != nil {
			return nil, err
		}
		if item.ID != id {
			return nil, i18n.Message("cannot read the saved profile")
		}
		records = append(records, item)
	}
	return records, nil
}

func (m Manager) findProfile(id string) (profile, bool, error) {
	current, err := m.readProfile()
	if err != nil || current.ID == id {
		return current, true, err
	}
	items, err := m.libraryProfiles()
	if err != nil {
		return profile{}, false, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, false, nil
		}
	}
	return profile{}, false, i18n.Message("Profile no longer exists")
}

func (m Manager) UpdateSavedProfile(ctx context.Context, id string) error {
	return m.withLock(func() error {
		item, active, err := m.findProfile(id)
		if err != nil {
			return err
		}
		if item.URL == "" {
			return i18n.Message("no subscription is configured")
		}
		return m.updateSavedProfile(ctx, item, active)
	})
}

func (m Manager) updateSavedProfile(ctx context.Context, item profile, active bool) error {
	next, err := downloadSubscription(ctx, item.URL, item.Name, item.CustomName)
	if err != nil {
		return err
	}
	next.ID, next.Schedule, next.Added = item.ID, item.Schedule, item.addedAt()
	if active {
		return m.applyProfile(ctx, next)
	}
	return m.validateAndSaveProfile(ctx, next)
}

func (m Manager) DeleteProfile(ctx context.Context, id string) error {
	return m.withLock(func() error {
		item, active, err := m.findProfile(id)
		if err != nil {
			return err
		}
		if item.Source == "" {
			return i18n.Message("Profile no longer exists")
		}
		if active {
			items, err := m.libraryProfiles()
			if err != nil {
				return err
			}
			switched := false
			for _, candidate := range items {
				if candidate.ID != id {
					if err := m.applyProfile(ctx, candidate); err != nil {
						return err
					}
					switched = true
					break
				}
			}
			if !switched {
				if err := m.applyProfile(ctx, profile{}); err != nil {
					return err
				}
			}
		}
		if id == "" {
			return nil
		}
		err = os.Remove(filepath.Join(m.libraryDir(), id))
		if os.IsNotExist(err) {
			return nil
		}
		return err
	})
}

func (p ProfileInfo) addedAt() time.Time {
	if p.Added.IsZero() {
		return p.Updated
	}
	return p.Added
}

type profileSummary struct {
	file os.FileInfo
	info ProfileInfo
}

type profileSummaryCache struct {
	mu    sync.Mutex
	files map[string]profileSummary
}

func (m Manager) ProfileLibrary() (ProfileInfo, []ProfileInfo, error) {
	cache := m.profileSummaries
	if cache != nil {
		cache.mu.Lock()
		defer cache.mu.Unlock()
	}
	var fresh map[string]profileSummary
	if cache != nil {
		fresh = make(map[string]profileSummary, len(cache.files))
	}
	read := func(path string) (ProfileInfo, error) {
		handle, err := os.Open(path)
		if err != nil {
			return ProfileInfo{}, err
		}
		defer handle.Close()
		file, err := handle.Stat()
		if err != nil {
			return ProfileInfo{}, err
		}
		if cache != nil {
			item, ok := cache.files[path]
			if ok && os.SameFile(item.file, file) && item.file.Size() == file.Size() &&
				item.file.ModTime().Equal(file.ModTime()) {
				fresh[path] = item
				return item.info, nil
			}
		}
		item, err := decodeSavedProfile(handle)
		if err != nil {
			return ProfileInfo{}, err
		}
		info := item.summary()
		if cache != nil {
			fresh[path] = profileSummary{file, info}
		}
		return info, nil
	}

	current, err := read(m.profilePath())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return ProfileInfo{}, nil, err
	}
	ids, err := m.libraryIDs()
	if err != nil {
		return current, nil, err
	}
	var result []ProfileInfo
	if current.Source != "" {
		card := current
		card.Active = true
		result = append(result, card)
	}
	for _, id := range ids {
		if id == current.ID {
			continue
		}
		item, err := read(filepath.Join(m.libraryDir(), id))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return current, nil, err
		}
		if item.ID != id {
			return current, nil, i18n.Message("cannot read the saved profile")
		}
		result = append(result, item)
	}
	if cache != nil {
		cache.files = fresh
	}

	sort.Slice(result, func(i, j int) bool {
		left, right := result[i].addedAt(), result[j].addedAt()
		if left.Equal(right) {
			return result[i].ID < result[j].ID
		}
		return left.Before(right)
	})
	return current, result, nil
}

func (m Manager) saveLibraryProfile(item profile) error {
	if err := os.MkdirAll(m.libraryDir(), 0o700); err != nil {
		return err
	}
	body, err := json.Marshal(item)
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(m.libraryDir(), item.ID), body)
}

func (m Manager) AddProfile(ctx context.Context, source, name string, local bool) error {
	if local && strings.HasPrefix(source, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			source = filepath.Join(home, source[2:])
		}
	}
	if err := m.Install(ctx); err != nil {
		return err
	}
	return m.withLock(func() error {
		var next profile
		var err error
		if local {
			next, err = loadProfileFile(source, name)
		} else {
			next, err = downloadSubscription(ctx, source, "", name)
		}
		if err != nil {
			return err
		}
		current, err := m.readProfile()
		if err != nil {
			return err
		}
		// IDs are stored as hex filenames.
		var id [16]byte
		rand.Read(id[:])
		next.ID = hex.EncodeToString(id[:])
		if current.Source == "" {
			return m.applyProfile(ctx, next)
		}
		return m.validateAndSaveProfile(ctx, next)
	})
}

func (m Manager) validateAndSaveProfile(ctx context.Context, next profile) error {
	config, err := m.renderConfig(next.YAML, instance{
		Controller: "127.0.0.1:9090", Proxy: "127.0.0.1:7890", Secret: "validation-only",
	})
	if err != nil {
		return err
	}
	if err := m.validateConfig(ctx, config); err != nil {
		return err
	}
	next.Updated = time.Now()
	if next.Added.IsZero() {
		next.Added = next.Updated
	}
	next.Nodes = countProfileNodes(next.YAML)
	return m.saveLibraryProfile(next)
}

func (m Manager) ActivateProfile(ctx context.Context, id string) error {
	return m.withLock(func() error {
		current, err := m.readProfile()
		if err != nil || id == current.ID {
			return err
		}
		items, err := m.libraryProfiles()
		if err != nil {
			return err
		}
		for _, item := range items {
			if item.ID != id {
				continue
			}
			if current.Source != "" {
				if current.ID == "" {
					var id [16]byte
					rand.Read(id[:])
					current.ID = hex.EncodeToString(id[:])
					body, err := json.Marshal(current)
					if err != nil {
						return err
					}
					if err := atomicWrite(m.profilePath(), body); err != nil {
						return err
					}
				}
				if err := m.saveLibraryProfile(current); err != nil {
					return err
				}
			}
			return m.applyProfile(ctx, item)
		}
		return i18n.Message("Profile no longer exists")
	})
}

func (m Manager) UpdateProfiles(ctx context.Context, dueOnly bool) (updated, failed int, err error) {
	err = m.withLock(func() error {
		current, err := m.readProfile()
		if err != nil {
			return err
		}
		items, err := m.libraryProfiles()
		if err != nil {
			return err
		}
		for index, item := range append([]profile{current}, items...) {
			if item.URL == "" || (index > 0 && item.ID == current.ID) {
				continue
			}
			if dueOnly && !item.UpdateDue(time.Now()) {
				continue
			}
			err := m.updateSavedProfile(ctx, item, index == 0)
			if err != nil {
				failed++
			} else {
				updated++
			}
		}
		return ctx.Err()
	})
	return
}
