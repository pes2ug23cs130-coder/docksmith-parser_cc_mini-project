package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type CacheEntry struct {
	LayerDigest string `json:"layer_digest"`
}

// GenerateCacheKey builds a deterministic hash from all inputs
func GenerateCacheKey(prevDigest string, instruction string, workDir string, env map[string]string, fileHashes map[string]string) string {

	var parts []string

	parts = append(parts, "prev="+prevDigest)
	parts = append(parts, "instruction="+instruction)
	parts = append(parts, "workdir="+workDir)

	// ENV sorted by key
	var envKeys []string
	for k := range env {
		envKeys = append(envKeys, k)
	}
	sort.Strings(envKeys)
	for _, k := range envKeys {
		parts = append(parts, "env="+k+"="+env[k])
	}

	// file hashes sorted by path (COPY only)
	var fileKeys []string
	for k := range fileHashes {
		fileKeys = append(fileKeys, k)
	}
	sort.Strings(fileKeys)
	for _, k := range fileKeys {
		parts = append(parts, "file="+k+"="+fileHashes[k])
	}

	combined := strings.Join(parts, "|")
	hash := sha256.Sum256([]byte(combined))
	return hex.EncodeToString(hash[:])
}

func cacheFilePath(key string) string {
	return filepath.Join(os.Getenv("HOME"), ".docksmith", "cache", key+".json")
}

// CheckCache returns digest if hit, empty string if miss
func CheckCache(key string) (string, bool) {

	path := cacheFilePath(key)

	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}

	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return "", false
	}

	// IMPORTANT: also verify layer file actually exists on disk
	layerPath := filepath.Join(
           os.Getenv("HOME"),
           ".docksmith",
           "layers",
           strings.TrimPrefix(entry.LayerDigest, "sha256:")+".tar",
         )
	if _, err := os.Stat(layerPath); err != nil {
		return "", false
	}

	return entry.LayerDigest, true
}

// SaveCache stores a cache entry after a MISS
func SaveCache(key string, layerDigest string) error {

	path := cacheFilePath(key)
	os.MkdirAll(filepath.Dir(path), 0755)

	entry := CacheEntry{LayerDigest: layerDigest}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

func PrintHit() {
    fmt.Print(" [CACHE HIT]")
}

func PrintMiss() {
    fmt.Print(" [CACHE MISS]")
}
