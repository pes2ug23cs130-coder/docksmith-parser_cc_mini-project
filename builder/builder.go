package builder

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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

	//
	// ✅ parser
	//
	instructions, state, err := parser.ParseDocksmithfile(docksmithfilePath)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

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

		//
		// ✅ FROM loads base manifest
		//
		case parser.FROM:
			fmt.Printf("Step %d/%d : FROM %s\n", stepNum, total, inst.Args)

			prevDigest = inst.Args

			// preserve created timestamp on warm rebuild
			baseManifest, err := runtime.LoadManifest(inst.Args)
			if err == nil {
				baseCreated = baseManifest.Created
			}

		//
		// ✅ COPY → cache → layer
		//
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
				state.WorkDir,
				state.Env,
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

		//
		// ✅ RUN → cache → isolated runtime → layer
		//
		case parser.RUN:
			fmt.Printf("Step %d/%d : RUN %s\n", stepNum, total, inst.Args)

			key := cache.GenerateCacheKey(
				prevDigest,
				"RUN "+inst.Args,
				state.WorkDir,
				state.Env,
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

			//
			// ✅ REQUIRED isolated execution
			//
			err := runtime.RunHostCommand(
				inst.Args,
				contextPath,
				envMapToSlice(state.Env),
				state.WorkDir,
			)
			if err != nil {
				return nil, fmt.Errorf("RUN failed: %w", err)
			}

			afterFiles, _ := layer.GetAllFiles(contextPath)
			changedFiles, afterState, _ := layer.GetChangedFiles(contextPath, afterFiles, beforeState)
			prevFileState = afterState

			if len(changedFiles) == 0 {
				continue
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

		case parser.WORKDIR:
			fmt.Printf("Step %d/%d : WORKDIR %s\n", stepNum, total, inst.Args)

		case parser.ENV:
			fmt.Printf("Step %d/%d : ENV %s\n", stepNum, total, inst.Args)

		case parser.CMD:
			fmt.Printf("Step %d/%d : CMD %s\n", stepNum, total, inst.Args)
		}
	}

	//
	// ✅ deterministic image digest from layer chain
	//
	h := sha256.New()
	sorted := append([]string{}, layerDigests...)
	sort.Strings(sorted)

	for _, l := range sorted {
		h.Write([]byte(l))
	}

	imageDigest := "sha256:" + hex.EncodeToString(h.Sum(nil))

	result := &BuildResult{
		ImageDigest:  imageDigest,
		LayerDigests: layerDigests,
		Env:          state.Env,
		Cmd:          parseCmd(state.Cmd),
		WorkDir:      state.WorkDir,
		CreatedBy:    createdBy,
		Created:      baseCreated,
	}

	return result, nil
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

	// JSON array style CMD
	if strings.HasPrefix(cmd, "[") && strings.HasSuffix(cmd, "]") {
		cmd = strings.TrimPrefix(cmd, "[")
		cmd = strings.TrimSuffix(cmd, "]")

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

	// shell style fallback
	return []string{"sh", "-c", cmd}
}