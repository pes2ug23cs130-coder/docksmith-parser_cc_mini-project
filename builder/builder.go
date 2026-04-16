package builder

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"docksmith/cache"
	"docksmith/layer"
	"docksmith/parser"
	"docksmith/runtime"
)

type BuildResult struct {
	ImageDigest  string
	LayerDigests []string
	Env          map[string]string
	Cmd          []string
	WorkDir      string
	CreatedBy    []string
	Created      string
}

func Build(contextPath string, imageName string, tag string, noCache bool) (*BuildResult, error) {
	docksmithfilePath := filepath.Join(contextPath, "Docksmithfile")

	instructions, parsedState, err := parser.ParseDocksmithfile(docksmithfilePath)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	currentWorkDir := ""
	currentEnv := make(map[string]string)
	currentCmd := parsedState.Cmd

	var prevDigest string
	var layerDigests []string
	var createdBy []string
	var baseCreated string

	cascadeMiss := false
	prevFileState := make(map[string]string)

	total := len(instructions)

	for i, inst := range instructions {
		stepNum := i + 1

		switch inst.Type {

		case parser.FROM:
			fmt.Printf("Step %d/%d : FROM %s\n", stepNum, total, inst.Args)
			prevDigest = inst.Args

			baseManifest, err := runtime.LoadManifest(inst.Args)
			if err == nil {
				baseCreated = baseManifest.Created
			}

		case parser.WORKDIR:
			fmt.Printf("Step %d/%d : WORKDIR %s\n", stepNum, total, inst.Args)
			currentWorkDir = inst.Args

		case parser.ENV:
			fmt.Printf("Step %d/%d : ENV %s\n", stepNum, total, inst.Args)
			parts := strings.SplitN(inst.Args, "=", 2)
			if len(parts) == 2 {
				currentEnv[parts[0]] = parts[1]
			}

		case parser.CMD:
			fmt.Printf("Step %d/%d : CMD %s\n", stepNum, total, inst.Args)
			currentCmd = inst.Args

		case parser.COPY:
			fmt.Printf("Step %d/%d : COPY %s\n", stepNum, total, inst.Args)

			allFiles, err := layer.GetAllFiles(contextPath)
			if err != nil {
				return nil, err
			}

			fileHashes := make(map[string]string)
			for _, f := range allFiles {
				h, err := layer.HashFile(filepath.Join(contextPath, f))
				if err != nil {
					return nil, err
				}
				fileHashes[f] = h
			}

			key := cache.GenerateCacheKey(
				prevDigest,
				"COPY "+inst.Args,
				currentWorkDir,
				currentEnv,
				fileHashes,
			)

			if !noCache && !cascadeMiss {
				if digest, hit := cache.CheckCache(key); hit {
					cache.PrintHit()
					prevDigest = digest
					layerDigests = append(layerDigests, digest)
					createdBy = append(createdBy, "COPY "+inst.Args)
					continue
				}
			}

			cache.PrintMiss()
			cascadeMiss = true

			changedFiles, newState, err := layer.GetChangedFiles(contextPath, allFiles, prevFileState)
			if err != nil {
				return nil, err
			}
			prevFileState = newState

			if len(changedFiles) == 0 {
				changedFiles = allFiles
			}

			layerObj, err := layer.CreateLayer(contextPath, changedFiles)
			if err != nil {
				return nil, err
			}

			if !noCache {
				cache.SaveCache(key, layerObj.Digest)
			}

			prevDigest = layerObj.Digest
			layerDigests = append(layerDigests, layerObj.Digest)
			createdBy = append(createdBy, "COPY "+inst.Args)

		case parser.RUN:
			fmt.Printf("Step %d/%d : RUN %s\n", stepNum, total, inst.Args)

			key := cache.GenerateCacheKey(
				prevDigest,
				"RUN "+inst.Args,
				currentWorkDir,
				currentEnv,
				nil,
			)

			if !noCache && !cascadeMiss {
				if digest, hit := cache.CheckCache(key); hit {
					cache.PrintHit()
					prevDigest = digest
					layerDigests = append(layerDigests, digest)
					createdBy = append(createdBy, "RUN "+inst.Args)
					continue
				}
			}

			cache.PrintMiss()
			cascadeMiss = true

			beforeFiles, _ := layer.GetAllFiles(contextPath)
			_, beforeState, _ := layer.GetChangedFiles(contextPath, beforeFiles, prevFileState)

			err := runtime.RunHostCommand(
				inst.Args,
				contextPath,
				envMapToSlice(currentEnv),
				currentWorkDir,
			)
			if err != nil {
				return nil, fmt.Errorf("RUN failed: %w", err)
			}

			afterFiles, _ := layer.GetAllFiles(contextPath)
			changedFiles, afterState, _ := layer.GetChangedFiles(contextPath, afterFiles, beforeState)
			prevFileState = afterState

			if len(changedFiles) == 0 {
				markerFile := ".docksmith_run_marker"
				markerPath := filepath.Join(contextPath, markerFile)

				err := os.WriteFile(markerPath, []byte(inst.Args), 0644)
				if err == nil {
					changedFiles = []string{markerFile}
				}
			}

			layerObj, err := layer.CreateLayer(contextPath, changedFiles)
			if err != nil {
				return nil, err
			}

			if !noCache {
				cache.SaveCache(key, layerObj.Digest)
			}

			prevDigest = layerObj.Digest
			layerDigests = append(layerDigests, layerObj.Digest)
			createdBy = append(createdBy, "RUN "+inst.Args)
		}
	}

	h := sha256.New()
	sorted := append([]string{}, layerDigests...)
	sort.Strings(sorted)

	for _, l := range sorted {
		h.Write([]byte(l))
	}

	imageDigest := "sha256:" + hex.EncodeToString(h.Sum(nil))

	return &BuildResult{
		ImageDigest:  imageDigest,
		LayerDigests: layerDigests,
		Env:          currentEnv,
		Cmd:          parseCmd(currentCmd),
		WorkDir:      currentWorkDir,
		CreatedBy:    createdBy,
		Created:      baseCreated,
	}, nil
}
func envMapToSlice(env map[string]string) []string {
	var result []string
	for k, v := range env {
		result = append(result, k+"="+v)
	}
	sort.Strings(result)
	return result
}

func parseCmd(cmd string) []string {
	cmd = strings.TrimSpace(cmd)
	cmd = strings.TrimPrefix(cmd, "[")
	cmd = strings.TrimSuffix(cmd, "]")

	if cmd == "" {
		return nil
	}

	parts := strings.Split(cmd, ",")
	var result []string

	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, `"`)
		if p != "" {
			result = append(result, p)
		}
	}

	return result
}
