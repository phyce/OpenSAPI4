// Package datapack reads the TruVoice voices' data pack (docs/truvoice/datapack.md).
package datapack

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sync"
)

const (
	Format        = "opensapi4-truvoice"
	FormatVersion = 1
)

type Manifest struct {
	Format        string            `json:"format"`
	FormatVersion int               `json:"formatVersion"`
	Files         map[string]string `json:"files"`
}

type Pack struct {
	rootPath string
	Manifest Manifest

	lock   sync.Mutex
	loaded map[string]any
}

func Open(rootPath string) (*Pack, error) {
	data, err := os.ReadFile(filepath.Join(rootPath, "manifest.json"))
	if err != nil {
		return nil, fmt.Errorf("read data pack manifest: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parse data pack manifest: %w", err)
	}
	if manifest.Format != Format {
		return nil, fmt.Errorf("data pack format is %q, expected %q", manifest.Format, Format)
	}
	if manifest.FormatVersion != FormatVersion {
		return nil, fmt.Errorf("data pack format version is %d, expected %d", manifest.FormatVersion, FormatVersion)
	}
	return &Pack{rootPath: rootPath, Manifest: manifest, loaded: map[string]any{}}, nil
}

func (pack *Pack) readFile(name string) ([]byte, error) {
	if _, listed := pack.Manifest.Files[name]; !listed {
		return nil, fmt.Errorf("data pack file %s is not in the manifest", name)
	}
	if !filepath.IsLocal(filepath.FromSlash(name)) || path.Clean(name) != name {
		return nil, fmt.Errorf("data pack file name %s is not a plain relative path", name)
	}
	data, err := os.ReadFile(filepath.Join(pack.rootPath, filepath.FromSlash(name)))
	if err != nil {
		return nil, fmt.Errorf("read data pack file: %w", err)
	}
	return data, nil
}

func load[T any](pack *Pack, name string, parse func([]byte) (*T, error)) (*T, error) {
	pack.lock.Lock()
	defer pack.lock.Unlock()
	if value, found := pack.loaded[name]; found {
		return value.(*T), nil
	}
	data, err := pack.readFile(name)
	if err != nil {
		return nil, err
	}
	value, err := parse(data)
	if err != nil {
		return nil, fmt.Errorf("data pack file %s: %w", name, err)
	}
	pack.loaded[name] = value
	return value, nil
}
