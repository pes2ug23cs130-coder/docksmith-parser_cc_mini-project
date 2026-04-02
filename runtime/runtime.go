package runtime

import (
	"archive/tar"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

type ImageManifest struct {
	Name   string            `json:"name"`
	Layers []string          `json:"layers"`
	Env    map[string]string `json:"env"`
	Cmd    string            `json:"cmd"`
	WorkDir string           `json:"workdir"`
}

// SaveManifest saves image manifest to ~/.docksmith/images/
func SaveManifest(imageName string, layers []string, env map[string]string, cmd string, workDir string) error {
	manifest := ImageManifest{
		Name:    imageName,
		Layers:  layers,
		Env:     env,
		Cmd:     cmd,
		WorkDir: workDir,
	}
	dir := filepath.Join(os.Getenv("HOME"), ".docksmith", "images")
	os.MkdirAll(dir, 0755)

	path := filepath.Join(dir, imageName+".json")
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// LoadManifest loads image manifest
func LoadManifest(imageName string) (*ImageManifest, error) {
	path := filepath.Join(os.Getenv("HOME"), ".docksmith", "images", imageName+".json")
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

// extractLayer extracts a tar layer into rootfs
func extractLayer(layerDigest string, rootfs string) error {
	layerPath := filepath.Join(os.Getenv("HOME"), ".docksmith", "layers", layerDigest+".tar")
	f, err := os.Open(layerPath)
	if err != nil {
		return fmt.Errorf("layer not found: %s", layerDigest)
	}
	defer f.Close()

	tr := tar.NewReader(f)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(rootfs, header.Name)
		os.MkdirAll(filepath.Dir(target), 0755)

		outFile, err := os.Create(target)
		if err != nil {
			return err
		}
		if _, err := io.Copy(outFile, tr); err != nil {
			outFile.Close()
			return err
		}
		outFile.Close()
	}
	return nil
}

// RunContainer extracts all layers and executes the command
func RunContainer(imageName string, overrideCmd string) error {
	manifest, err := LoadManifest(imageName)
	if err != nil {
		return err
	}

	// create temp rootfs
	rootfs, err := os.MkdirTemp("", "docksmith-rootfs-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(rootfs)

	fmt.Println("Extracting layers...")
	for _, digest := range manifest.Layers {
		if err := extractLayer(digest, rootfs); err != nil {
			return fmt.Errorf("failed to extract layer %s: %w", digest[:12], err)
		}
		fmt.Printf("  extracted: sha256:%s...\n", digest[:12])
	}

	// determine command to run
	cmdStr := manifest.Cmd
	if overrideCmd != "" {
		cmdStr = overrideCmd
	}
	if cmdStr == "" {
		return fmt.Errorf("no command specified")
	}

	fmt.Printf("Running: %s\n\n", cmdStr)

	cmd := exec.Command("sh", "-c", cmdStr)

	// set working directory inside rootfs
	workDir := filepath.Join(rootfs, manifest.WorkDir)
	os.MkdirAll(workDir, 0755)
	cmd.Dir = workDir

	// set environment variables
	cmd.Env = os.Environ()
	for k, v := range manifest.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// ListImages lists all saved images
func ListImages() error {
	dir := filepath.Join(os.Getenv("HOME"), ".docksmith", "images")
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Println("No images found.")
		return nil
	}

	fmt.Printf("%-30s %-10s\n", "IMAGE NAME", "LAYERS")
	fmt.Println("----------------------------------------")
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".json" {
			continue
		}
		name := e.Name()[:len(e.Name())-5]
		manifest, err := LoadManifest(name)
		if err != nil {
			continue
		}
		fmt.Printf("%-30s %-10d\n", name, len(manifest.Layers))
	}
	return nil
}

// RemoveImage deletes an image manifest
func RemoveImage(imageName string) error {
	path := filepath.Join(os.Getenv("HOME"), ".docksmith", "images", imageName+".json")
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("image not found: %s", imageName)
	}
	fmt.Printf("Deleted image: %s\n", imageName)
	return nil
}
