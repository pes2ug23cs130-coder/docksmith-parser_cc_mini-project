package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"docksmith/layer"
)

type Config struct {
	Env        []string `json:"Env"`
	Cmd        []string `json:"Cmd"`
	WorkingDir string   `json:"WorkingDir"`
}

type LayerInfo struct {
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
	CreatedBy string `json:"createdBy"`
}

type ImageManifest struct {
	Name    string      `json:"name"`
	Tag     string      `json:"tag"`
	Digest  string      `json:"digest"`
	Created string      `json:"created"`
	Config  Config      `json:"config"`
	Layers  []LayerInfo `json:"layers"`
}

// RunIsolated runs a command inside a chroot-ed rootfs.
// Used by BOTH the build RUN step and docksmith run.
func RunIsolated(rootfs string, cmdArgs []string, env []string, workdir string) error {
	if len(cmdArgs) == 0 {
		return fmt.Errorf("no command specified")
	}

	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Chroot: rootfs,
	}

	cmd.Env = append(os.Environ(), env...)
	cmd.Dir = workdir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Run()
}

// SaveManifest saves the image manifest to ~/.docksmith/images/<imageName>.json
func SaveManifest(
	imageName string,
	tag string,
	imageDigest string,
	layerDigests []string,
	env map[string]string,
	cmd []string,
	workDir string,
	createdBy []string,
) error {
	var envList []string
	for k, v := range env {
		envList = append(envList, k+"="+v)
	}
	sort.Strings(envList)

	var layers []LayerInfo
	for i, digest := range layerDigests {
		layerPath := filepath.Join(
			os.Getenv("HOME"),
			".docksmith",
			"layers",
			strings.TrimPrefix(digest, "sha256:")+".tar",
		)

		info, err := os.Stat(layerPath)
		if err != nil {
			return err
		}

		cb := ""
		if i < len(createdBy) {
			cb = createdBy[i]
		}

		layers = append(layers, LayerInfo{
			Digest:    digest,
			Size:      info.Size(),
			CreatedBy: cb,
		})
	}

	manifest := ImageManifest{
		Name:    imageName,
		Tag:     tag,
		Digest:  imageDigest,
		Created: time.Now().UTC().Format(time.RFC3339),
		Config: Config{
			Env:        envList,
			Cmd:        cmd,
			WorkingDir: workDir,
		},
		Layers: layers,
	}

	dir := filepath.Join(os.Getenv("HOME"), ".docksmith", "images")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	path := filepath.Join(dir, imageName+".json")
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// LoadManifest reads an image manifest from ~/.docksmith/images/
func LoadManifest(imageName string) (*ImageManifest, error) {
	path := filepath.Join(
		os.Getenv("HOME"),
		".docksmith",
		"images",
		imageName+".json",
	)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("image not found: %s", imageName)
	}

	var manifest ImageManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}

	return &manifest, nil
}

// RunContainerWithEnv extracts all image layers and runs the container command.
func RunContainerWithEnv(imageName string, overrideCmd string, extraEnv []string) error {
	manifest, err := LoadManifest(imageName)
	if err != nil {
		return err
	}

	rootfs, err := os.MkdirTemp("", "docksmith-rootfs-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(rootfs)

	fmt.Println("Extracting layers...")
	for _, layerInfo := range manifest.Layers {
		if err := layer.ExtractLayer(layerInfo.Digest, rootfs); err != nil {
			return fmt.Errorf("failed to extract %s: %w", layerInfo.Digest, err)
		}
		fmt.Printf("  extracted: %s\n", layerInfo.Digest)
	}

	// ✅ FIX: Determine cmdArgs correctly.
	// manifest.Config.Cmd is already []string — use it directly.
	// If overrideCmd is provided as a string, wrap it in sh -c.
	var cmdArgs []string
	if overrideCmd != "" {
		cmdArgs = []string{"sh", "-c", overrideCmd}
	} else {
		cmdArgs = manifest.Config.Cmd
	}

	if len(cmdArgs) == 0 {
		return fmt.Errorf("no command specified")
	}

	env := append(manifest.Config.Env, extraEnv...)

	workDir := manifest.Config.WorkingDir
	if workDir == "" {
		workDir = "/"
	}

	fmt.Printf("Running: %s\n\n", strings.Join(cmdArgs, " "))

	// ✅ FIX: Always use RunHostCommand (host shell) for the demo/mini-project.
	// RunIsolated with Chroot requires root + a full rootfs with /bin/sh inside it.
	// Since this project uses a simulated rootfs (no real Alpine binaries),
	// we run on the host and point workdir into the extracted rootfs.
	return RunHostCommand(
		strings.Join(cmdArgs[2:], " "), // extract the actual shell command (index 2 onward)
		rootfs,
		env,
		workDir,
	)
}

// ListImages prints all saved images.
func ListImages() error {
	dir := filepath.Join(os.Getenv("HOME"), ".docksmith", "images")

	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Println("No images found.")
		return nil
	}

	fmt.Printf("%-20s %-10s %-10s\n", "IMAGE", "TAG", "LAYERS")
	fmt.Println("------------------------------------------------")

	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".json" {
			continue
		}

		name := strings.TrimSuffix(e.Name(), ".json")
		manifest, err := LoadManifest(name)
		if err != nil {
			continue
		}

		fmt.Printf(
			"%-20s %-10s %-10d\n",
			manifest.Name,
			manifest.Tag,
			len(manifest.Layers),
		)
	}

	return nil
}

// RemoveImage deletes an image manifest.
func RemoveImage(imageName string) error {
	path := filepath.Join(
		os.Getenv("HOME"),
		".docksmith",
		"images",
		imageName+".json",
	)

	if err := os.Remove(path); err != nil {
		return fmt.Errorf("image not found: %s", imageName)
	}

	fmt.Printf("Deleted image: %s\n", imageName)
	return nil
}

// RunHostCommand runs a shell command on the host machine, with workdir set
// inside the extracted rootfs. This is the safe fallback for demo builds
// that don't have real Alpine binaries in the rootfs.
func RunHostCommand(command string, contextPath string, env []string, workdir string) error {
	// ✅ FIX: Build the correct host-side working directory.
	// rootfs/app → we join contextPath + workdir (strip leading slash).
	hostWorkDir := contextPath
	if workdir != "" && workdir != "/" {
		hostWorkDir = filepath.Join(contextPath, strings.TrimPrefix(workdir, "/"))
	}

	// Auto-create the workdir (mirrors Docker WORKDIR behaviour)
	if err := os.MkdirAll(hostWorkDir, 0755); err != nil {
		return err
	}

	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = hostWorkDir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Run()
}