package main

import (
	"docksmith/builder"
	"docksmith/runtime"
	"fmt"
	"os"
)

func main() {

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
		runtime.ListImages()

	case "rmi":
		if len(os.Args) < 3 {
			fmt.Println("Usage: docksmith rmi <image-name>")
			return
		}
		runtime.RemoveImage(os.Args[2])

	default:
		fmt.Printf("Unknown command: %s\n", command)
		printHelp()
	}
}

func runBuild() {
	imageName := "myapp:latest"
	contextPath := "."
	noCache := false

	args := os.Args[2:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-t":
			if i+1 < len(args) {
				imageName = args[i+1]
				i++
			}
		case "--no-cache":
			noCache = true
		default:
			contextPath = args[i]
		}
	}

	fmt.Printf("Building image: %s from context: %s\n\n", imageName, contextPath)

	layers, state, err := builder.BuildAndReturn(contextPath, imageName, noCache)
	if err != nil {
		fmt.Println("Build Error:", err)
		return
	}

	err = runtime.SaveManifest(imageName, layers, state.Env, state.Cmd, state.WorkDir)
	if err != nil {
		fmt.Println("Error saving manifest:", err)
		return
	}

	fmt.Printf("\nSuccessfully built image: %s\n", imageName)
}

func runContainer() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: docksmith run <image-name> [command]")
		return
	}

	imageName := os.Args[2]
	overrideCmd := ""
	if len(os.Args) > 3 {
		overrideCmd = os.Args[3]
	}

	err := runtime.RunContainer(imageName, overrideCmd)
	if err != nil {
		fmt.Println("Run Error:", err)
	}
}

func printHelp() {
	fmt.Println("Usage: docksmith <command> [options]")
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("  build [-t image:tag] [--no-cache] [context]   Build an image")
	fmt.Println("  run <image-name> [command]                     Run a container")
	fmt.Println("  images                                         List all images")
	fmt.Println("  rmi <image-name>                               Remove an image")
}
