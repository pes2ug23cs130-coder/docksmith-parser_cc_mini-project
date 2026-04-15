package cmd

import (
	"fmt"
	"os"
	"strings"

	"docksmith/builder"
	"docksmith/runtime"
)

func Execute() {
	if len(os.Args) < 2 {
		printHelp()
		return
	}

	command := os.Args[1]

	switch command {
	case "build":
		runBuild()

	case "run":
		runContainer()

	case "images":
		if err := runtime.ListImages(); err != nil {
			fmt.Println("Images Error:", err)
		}

	case "rmi":
		if len(os.Args) < 3 {
			fmt.Println("Usage: docksmith rmi <image-name>")
			return
		}
		if err := runtime.RemoveImage(os.Args[2]); err != nil {
			fmt.Println("Remove Error:", err)
		}

	default:
		fmt.Printf("Unknown command: %s\n", command)
		printHelp()
	}
}

func runBuild() {
	imageName := "myapp"
	tag := "latest"
	contextPath := "."
	noCache := false

	args := os.Args[2:]

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-t":
			if i+1 < len(args) {
				full := args[i+1]
				parts := strings.Split(full, ":")
				imageName = parts[0]
				if len(parts) > 1 {
					tag = parts[1]
				}
				i++
			}

		case "--no-cache":
			noCache = true

		default:
			contextPath = args[i]
		}
	}

	fmt.Printf("Building image: %s:%s\n", imageName, tag)
	fmt.Printf("Context: %s\n\n", contextPath)

	//
	// ✅ REQUIRED PIPELINE FLOW
	// parser → cache → layer → manifest → runtime
	//
	result, err := builder.Build(contextPath, imageName, tag, noCache)
	if err != nil {
		fmt.Println("Build Error:", err)
		return
	}

	err = runtime.SaveManifest(
		imageName,
		tag,
		result.ImageDigest,
		result.LayerDigests,
		result.Env,
		result.Cmd,
		result.WorkDir,
		result.CreatedBy,
	)
	if err != nil {
		fmt.Println("Manifest Error:", err)
		return
	}

	fmt.Printf("\nSuccessfully built %s:%s\n", imageName, tag)
	fmt.Printf("Image ID: %s\n", trimDigest(result.ImageDigest))
}

func runContainer() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: docksmith run <image-name> [-e KEY=VALUE] [command]")
		return
	}

	imageName := os.Args[2]
	overrideCmd := ""
	var envOverrides []string

	args := os.Args[3:]

	for i := 0; i < len(args); i++ {
		if args[i] == "-e" && i+1 < len(args) {
			envOverrides = append(envOverrides, args[i+1])
			i++
			continue
		}

		overrideCmd = strings.Join(args[i:], " ")
		break
	}

	err := runtime.RunContainerWithEnv(imageName, overrideCmd, envOverrides)
	if err != nil {
		fmt.Println("Run Error:", err)
	}
}

func trimDigest(d string) string {
	d = strings.TrimPrefix(d, "sha256:")
	if len(d) > 12 {
		return d[:12]
	}
	return d
}

func printHelp() {
	fmt.Println("Usage: docksmith <command> [options]")
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("  build [-t name:tag] [--no-cache] [context]")
	fmt.Println("  run <image-name> [-e KEY=VALUE] [command]")
	fmt.Println("  images")
	fmt.Println("  rmi <image-name>")
}