package layer

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Layer struct {
	Digest string
	Path   string
}

// CreateLayer creates an immutable deterministic tar layer
func CreateLayer(basePath string, files []string) (*Layer, error) {
	sort.Strings(files)

	tmpTar := "layer.tar"

	tarFile, err := os.Create(tmpTar)
	if err != nil {
		return nil, err
	}
	defer tarFile.Close()

	tw := tar.NewWriter(tarFile)
	defer tw.Close()

	for _, file := range files {
		fullPath := filepath.Join(basePath, file)

		info, err := os.Stat(fullPath)
		if err != nil {
			return nil, err
		}

		// only regular files
		if !info.Mode().IsRegular() {
			continue
		}

		data, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, err
		}

		// deterministic tar header
		header := &tar.Header{
			Name:       filepath.ToSlash(file),
			Mode:       int64(info.Mode().Perm()),
			Size:       int64(len(data)),
			ModTime:    time.Unix(0, 0),
			AccessTime: time.Unix(0, 0),
			ChangeTime: time.Unix(0, 0),
		}

		if err := tw.WriteHeader(header); err != nil {
			return nil, err
		}

		if _, err := tw.Write(data); err != nil {
			return nil, err
		}
	}

	// flush writer fully before hashing
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := tarFile.Close(); err != nil {
		return nil, err
	}

	// compute sha256 digest
	hashFile, err := os.Open(tmpTar)
	if err != nil {
		return nil, err
	}
	defer hashFile.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, hashFile); err != nil {
		return nil, err
	}

	digest := "sha256:" + hex.EncodeToString(hasher.Sum(nil))

	layerPath := filepath.Join(
		os.Getenv("HOME"),
		".docksmith",
		"layers",
		strings.TrimPrefix(digest, "sha256:")+".tar",
	)

	if err := os.MkdirAll(filepath.Dir(layerPath), 0755); err != nil {
		return nil, err
	}

	// rewind temp tar for copy
	if _, err := hashFile.Seek(0, 0); err != nil {
		return nil, err
	}

	dstFile, err := os.Create(layerPath)
	if err != nil {
		return nil, err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, hashFile); err != nil {
		return nil, err
	}

	_ = os.Remove(tmpTar)

	return &Layer{
		Digest: digest,
		Path:   layerPath,
	}, nil
}

// GetAllFiles returns all valid files in sorted order
func GetAllFiles(basePath string) ([]string, error) {
	var files []string

	err := filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// skip docksmith internals and git
		if strings.Contains(path, ".docksmith") || strings.Contains(path, ".git") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(basePath, path)
		if err != nil {
			return err
		}

		files = append(files, filepath.ToSlash(relPath))
		return nil
	})

	sort.Strings(files)
	return files, err
}

// HashFile computes sha256 of a file
func HashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// GetChangedFiles detects changed files using sha256
func GetChangedFiles(basePath string, files []string, prev map[string]string) ([]string, map[string]string, error) {
	current := make(map[string]string)
	var changed []string

	sort.Strings(files)

	for _, file := range files {
		fullPath := filepath.Join(basePath, file)

		hash, err := HashFile(fullPath)
		if err != nil {
			return nil, nil, err
		}

		current[file] = hash

		if prevHash, ok := prev[file]; !ok || prevHash != hash {
			changed = append(changed, file)
		}
	}

	return changed, current, nil
}

// ExtractLayer safely extracts a layer tar into rootfs
func ExtractLayer(layerDigest, rootfs string) error {
	layerPath := filepath.Join(
		os.Getenv("HOME"),
		".docksmith",
		"layers",
		strings.TrimPrefix(layerDigest, "sha256:")+".tar",
	)

	f, err := os.Open(layerPath)
	if err != nil {
		return err
	}
	defer f.Close()

	tr := tar.NewReader(f)

	rootfsClean := filepath.Clean(rootfs)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		clean := filepath.Clean(hdr.Name)
		target := filepath.Join(rootfsClean, clean)

		// tar-slip protection
		if !strings.HasPrefix(target, rootfsClean) {
			return fmt.Errorf("unsafe tar path: %s", hdr.Name)
		}

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}

		out, err := os.Create(target)
		if err != nil {
			return err
		}

		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return err
		}
		out.Close()
	}

	return nil
}

// ExecuteRunCommand runs a RUN instruction during build.
// It inherits the host environment (so PATH, sh etc. are available)
// and appends any ENV variables declared in the Docksmithfile.
func ExecuteRunCommand(command string, workDir string, env map[string]string) error {
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = workDir

	// ✅ FIX: inherit host env so PATH and sh are available,
	// then layer on any ENV vars from the Docksmithfile
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}