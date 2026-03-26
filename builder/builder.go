package builder

import (
	"docksmith/cache"
	"docksmith/layer"
	"docksmith/parser"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func Build(contextPath string, imageName string, noCache bool) error {

	docksmithfilePath := filepath.Join(contextPath, "Docksmithfile")

	instructions, state, err := parser.ParseDocksmithfile(docksmithfilePath)
	if err != nil {
		return fmt.Errorf("parse error: %w", err)
	}

	var prevDigest string
	var layers []string
	cascadeMiss := false
	prevFileState := make(map[string]string)

	total := len(instructions)

	for i, inst := range instructions {

		stepNum := i + 1

		switch inst.Type {

		case parser.FROM:
			fmt.Printf("Step %d/%d : FROM %s\n", stepNum, total, inst.Args)
			// use FROM image name as base digest seed
			prevDigest = inst.Args

		case parser.COPY:

			fmt.Printf("Step %d/%d : COPY %s\n", stepNum, total, inst.Args)

			// get all files from context
			allFiles, err := layer.GetAllFiles(contextPath)
			if err != nil {
				return err
			}

			// compute file hashes for cache key
			fileHashes := make(map[string]string)
			for _, f := range allFiles {
				h, err := layer.HashFile(filepath.Join(contextPath, f))
				if err != nil {
					return err
				}
				fileHashes[f] = h
			}

			// generate cache key
			key := cache.GenerateCacheKey(prevDigest, string(inst.Type)+" "+inst.Args, state.WorkDir, state.Env, fileHashes)

			if !noCache && !cascadeMiss {
				if digest, hit := cache.CheckCache(key); hit {
					cache.PrintHit(inst.Args)
					prevDigest = digest
					layers = append(layers, digest)
					continue
				}
			}

			// CACHE MISS
			cascadeMiss = true
			cache.PrintMiss(inst.Args)

			// get changed files (delta)
			changedFiles, newState, err := layer.GetChangedFiles(contextPath, allFiles, prevFileState)
			if err != nil {
				return err
			}
			prevFileState = newState

			if len(changedFiles) == 0 {
				changedFiles = allFiles
			}

			layerObj, err := layer.CreateLayer(contextPath, changedFiles)
			if err != nil {
				return err
			}

			if !noCache {
				cache.SaveCache(key, layerObj.Digest)
			}

			prevDigest = layerObj.Digest
			layers = append(layers, layerObj.Digest)

		case parser.RUN:

			fmt.Printf("Step %d/%d : RUN %s\n", stepNum, total, inst.Args)

			key := cache.GenerateCacheKey(prevDigest, string(inst.Type)+" "+inst.Args, state.WorkDir, state.Env, nil)

			if !noCache && !cascadeMiss {
				if digest, hit := cache.CheckCache(key); hit {
					cache.PrintHit(inst.Args)
					prevDigest = digest
					layers = append(layers, digest)
					continue
				}
			}

			// CACHE MISS
			cascadeMiss = true
			cache.PrintMiss(inst.Args)

			// snapshot before
			beforeFiles, _ := layer.GetAllFiles(contextPath)
			_, beforeState, _ := layer.GetChangedFiles(contextPath, beforeFiles, prevFileState)

			// execute command
			err := layer.ExecuteRunCommand(inst.Args, contextPath, state.Env)
			if err != nil {
				return fmt.Errorf("RUN failed: %w", err)
			}

			// snapshot after
			afterFiles, _ := layer.GetAllFiles(contextPath)
			changedFiles, afterState, _ := layer.GetChangedFiles(contextPath, afterFiles, beforeState)
			prevFileState = afterState

			if len(changedFiles) == 0 {
				fmt.Println("    (no file changes from RUN)")
				continue
			}

			layerObj, err := layer.CreateLayer(contextPath, changedFiles)
			if err != nil {
				return err
			}

			if !noCache {
				cache.SaveCache(key, layerObj.Digest)
			}

			prevDigest = layerObj.Digest
			layers = append(layers, layerObj.Digest)

		case parser.WORKDIR:
			fmt.Printf("Step %d/%d : WORKDIR %s\n", stepNum, total, inst.Args)

		case parser.ENV:
			fmt.Printf("Step %d/%d : ENV %s\n", stepNum, total, inst.Args)

		case parser.CMD:
			fmt.Printf("Step %d/%d : CMD %s\n", stepNum, total, inst.Args)
		}
	}

	// print summary
	fmt.Println("\n Build complete")
	fmt.Println("Layers:")
	for _, l := range layers {
		fmt.Println("  sha256:" + l[:12] + "...")
	}
	fmt.Println("WORKDIR:", state.WorkDir)
	fmt.Println("ENV:", strings.Join(envMapToSlice(state.Env), ", "))
	fmt.Println("CMD:", state.Cmd)

	_ = os.Getenv("HOME") // suppress unused import warning

	return nil
}

func envMapToSlice(env map[string]string) []string {
	var result []string
	for k, v := range env {
		result = append(result, k+"="+v)
	}
	return result
}
