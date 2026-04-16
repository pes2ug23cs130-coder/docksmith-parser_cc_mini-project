package layer

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Layer struct {
	Digest string
	Path   string
}

func CreateLayer(basePath string, changedFiles []string) (*Layer, error) {
	hash := sha256.New()

	for _, file := range changedFiles {
		hash.Write([]byte(file))
		fileHash, err := HashFile(filepath.Join(basePath, file))
		if err != nil {
			return nil, err
		}
		hash.Write([]byte(fileHash))
	}

	digest := "sha256:" + hex.EncodeToString(hash.Sum(nil))

	layerDir := filepath.Join(os.Getenv("HOME"), ".docksmith", "layers")
	err := os.MkdirAll(layerDir, 0755)
	if err != nil {
		return nil, err
	}

	layerPath := filepath.Join(
		layerDir,
		strings.TrimPrefix(digest, "sha256:")+".tar",
	)

	file, err := os.Create(layerPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	tw := tar.NewWriter(file)
	defer tw.Close()

	sort.Strings(changedFiles)

	for _, relPath := range changedFiles {
		fullPath := filepath.Join(basePath, relPath)

		info, err := os.Stat(fullPath)
		if err != nil {
			continue
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return nil, err
		}

		header.Name = filepath.ToSlash(relPath)

		err = tw.WriteHeader(header)
		if err != nil {
			return nil, err
		}

		src, err := os.Open(fullPath)
		if err != nil {
			return nil, err
		}

		_, err = io.Copy(tw, src)
		src.Close()
		if err != nil {
			return nil, err
		}
	}

	return &Layer{
		Digest: digest,
		Path:   layerPath,
	}, nil
}

func GetChangedFiles(basePath string, currentFiles []string, prev map[string]string) ([]string, map[string]string, error) {
	currentState := make(map[string]string)
	var changed []string

	for _, file := range currentFiles {
		fullPath := filepath.Join(basePath, file)

		hash, err := HashFile(fullPath)
		if err != nil {
			continue
		}

		currentState[file] = hash

		if prevHash, ok := prev[file]; !ok || prevHash != hash {
			changed = append(changed, file)
		}
	}

	sort.Strings(changed)
	return changed, currentState, nil
}

func GetAllFiles(basePath string) ([]string, error) {
	var files []string

	err := filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		name := info.Name()

		if info.IsDir() {
			if name == ".git" ||
				name == "layers" ||
				strings.HasPrefix(name, ".docksmith") {
				return filepath.SkipDir
			}
			return nil
		}

		relPath, err := filepath.Rel(basePath, path)
		if err != nil {
			return err
		}

		base := filepath.Base(relPath)

		// ignore generated files + synthetic run marker
		if base == "docksmith" ||
			base == ".docksmith_run_marker" ||
			strings.HasSuffix(base, ".tar") ||
			strings.HasSuffix(base, ".json") {
			return nil
		}

		files = append(files, filepath.ToSlash(relPath))
		return nil
	})

	sort.Strings(files)
	return files, err
}

func HashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	_, err = io.Copy(hash, file)
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func ExtractLayer(layerPath string, destDir string) error {
	file, err := os.Open(layerPath)
	if err != nil {
		return err
	}
	defer file.Close()

	tr := tar.NewReader(file)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		targetPath := filepath.Join(destDir, header.Name)

		err = os.MkdirAll(filepath.Dir(targetPath), 0755)
		if err != nil {
			return err
		}

		outFile, err := os.Create(targetPath)
		if err != nil {
			return err
		}

		_, err = io.Copy(outFile, tr)
		outFile.Close()
		if err != nil {
			return err
		}
	}

	return nil
}
