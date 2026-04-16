package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type LayerInfo struct {
	Digest string `json:"digest"`
}

type Config struct {
	Env        []string `json:"Env"`
	Cmd        []string `json:"Cmd"`
	WorkingDir string   `json:"WorkingDir"`
}

type Manifest struct {
	Name    string      `json:"name"`
	Tag     string      `json:"tag"`
	Digest  string      `json:"digest"`
	Config  Config      `json:"config"`
	Layers  []LayerInfo `json:"layers"`
	Created string      `json:"created"`
}

func LoadManifest(imageName string) (*Manifest, error) {
	parts := strings.Split(imageName, ":")
	name := parts[0]

	manifestPath := filepath.Join(
		os.Getenv("HOME"),
		".docksmith",
		"images",
		name+"_latest.json",
	)

	file, err := os.Open(manifestPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var manifest Manifest
	err = json.NewDecoder(file).Decode(&manifest)
	if err != nil {
		return nil, err
	}

	return &manifest, nil
}

func SaveManifest(
	imageName string,
	tag string,
	digest string,
	layerDigests []string,
	env map[string]string,
	cmd []string,
	workDir string,
	createdBy []string,
) error {
	imageDir := filepath.Join(os.Getenv("HOME"), ".docksmith", "images")
	err := os.MkdirAll(imageDir, 0755)
	if err != nil {
		return err
	}

	var envSlice []string
	for k, v := range env {
		envSlice = append(envSlice, k+"="+v)
	}

	var layers []LayerInfo
	for _, d := range layerDigests {
		layers = append(layers, LayerInfo{Digest: d})
	}

	manifest := Manifest{
		Name:   imageName,
		Tag:    tag,
		Digest: digest,
		Config: Config{
			Env:        envSlice,
			Cmd:        cmd,
			WorkingDir: workDir,
		},
		Layers: layers,
	}

	manifestPath := filepath.Join(
		imageDir,
		imageName+"_"+tag+".json",
	)

	file, err := os.Create(manifestPath)
	if err != nil {
		return err
	}
	defer file.Close()

	return json.NewEncoder(file).Encode(manifest)
}

func ListImages() error {
	imageDir := filepath.Join(os.Getenv("HOME"), ".docksmith", "images")
	files, err := os.ReadDir(imageDir)
	if err != nil {
		return err
	}

	fmt.Printf("%-20s %-10s %-10s\n", "IMAGE", "TAG", "LAYERS")
	fmt.Println("------------------------------------------------")

	for _, f := range files {
		var m Manifest
		file, err := os.Open(filepath.Join(imageDir, f.Name()))
		if err != nil {
			continue
		}
		_ = json.NewDecoder(file).Decode(&m)
		file.Close()

		fmt.Printf("%-20s %-10s %-10d\n",
			m.Name,
			m.Tag,
			len(m.Layers),
		)
	}

	return nil
}

func RemoveImage(imageName string) error {
	parts := strings.Split(imageName, ":")
	name := parts[0]

	manifestPath := filepath.Join(
		os.Getenv("HOME"),
		".docksmith",
		"images",
		name+"_latest.json",
	)

	return os.Remove(manifestPath)
}

func RunContainerWithEnv(imageName string, overrideCmd string, extraEnv []string) error {
	manifest, err := LoadManifest(imageName)
	if err != nil {
		return err
	}

	env := append([]string{}, manifest.Config.Env...)
	env = append(env, extraEnv...)

	command := overrideCmd
	if command == "" {
		command = strings.Join(manifest.Config.Cmd, " ")
	}

	return RunHostCommand(
		command,
		".",
		env,
		manifest.Config.WorkingDir,
	)
}

func RunHostCommand(command string, contextPath string, env []string, workDir string) error {
	cmd := exec.Command("sh", "-c", command)

	if workDir != "" {
		if filepath.IsAbs(workDir) {
			workDir = filepath.Join(contextPath, strings.TrimPrefix(workDir, "/"))
		}
		cmd.Dir = workDir
	} else {
		cmd.Dir = contextPath
	}

	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Run()
}
