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
	"time"
	"os/exec"
)

type Layer struct {
	Digest string
	Path   string
}

func CreateLayer(basePath string, files []string) (*Layer, error) {

	tmpTar := "layer.tar"

	tarFile, err := os.Create(tmpTar)
	if err != nil {
		return nil, err
	}

	tw := tar.NewWriter(tarFile)

	sort.Strings(files)

	for _, file := range files {

		fullPath := filepath.Join(basePath, file)

		info, err := os.Stat(fullPath)
		if err != nil {
			return nil, err
		}

		// skip non-regular files
		if !info.Mode().IsRegular() {
			continue
		}

		// read file first (safe)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, err
		}

		// create header ONCE
		header := &tar.Header{
			Name:    file,
			Mode:    int64(info.Mode().Perm()),
			Size:    int64(len(data)),
			ModTime: time.Unix(0, 0),
		}

		if err := tw.WriteHeader(header); err != nil {
			return nil, err
		}

		// write file data
		if _, err := tw.Write(data); err != nil {
			return nil, err
		}
	}

	// close tar properly
	if err := tw.Close(); err != nil {
		return nil, err
	}

	if err := tarFile.Close(); err != nil {
		return nil, err
	}

	// compute hash
	hashFile, err := os.Open(tmpTar)
	if err != nil {
		return nil, err
	}
	defer hashFile.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, hashFile); err != nil {
		return nil, err
	}

	digest := hex.EncodeToString(hasher.Sum(nil))

	// save layer
	layerPath := filepath.Join(os.Getenv("HOME"), ".docksmith", "layers", digest+".tar")
	os.MkdirAll(filepath.Dir(layerPath), 0755)

	srcFile, err := os.Open(tmpTar)
	if err != nil {
		return nil, err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(layerPath)
	if err != nil {
		return nil, err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return nil, err
	}

	os.Remove(tmpTar)

	return &Layer{
		Digest: digest,
		Path:   layerPath,
	}, nil
}

func GetAllFiles(basePath string) ([]string, error) {

	var files []string

	err := filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// skip unwanted dirs
		if strings.Contains(path, ".docksmith") || strings.Contains(path, ".git") {
			return nil
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(basePath, path)
		if err != nil {
			return err
		}

		files = append(files, relPath)
		return nil
	})

	return files, err
}
func HashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}
func GetChangedFiles(basePath string, files []string, prev map[string]string) ([]string, map[string]string, error) {

	current := make(map[string]string)
	var changed []string

	for _, file := range files {

		fullPath := filepath.Join(basePath, file)

		hash, err := HashFile(fullPath)
		if err != nil {
			return nil, nil, err
		}

		current[file] = hash

		// new file OR modified file
		if prevHash, ok := prev[file]; !ok || prevHash != hash {
			changed = append(changed, file)
		}
	}

	return changed, current, nil
}
func ExecuteRunCommand(command string, workDir string, env map[string]string) error {

	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = workDir

	// set environment variables
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
