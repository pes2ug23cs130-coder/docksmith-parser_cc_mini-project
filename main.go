package main

import (
	"docksmith/layer"
	"docksmith/parser"
	"fmt"
)

var prevState = make(map[string]string)

func main() {

	instructions, state, err := parser.ParseDocksmithfile("Docksmithfile")
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Println("Parsed Instructions:")
	for _, inst := range instructions {
		fmt.Printf("Line %d: %s %s\n", inst.Line, inst.Type, inst.Args)
	}

	var files []string

	for _, inst := range instructions {

		if inst.Type == parser.COPY {

			allFiles, err := layer.GetAllFiles(".")
			if err != nil {
				fmt.Println("Error:", err)
				return
			}
	
			changedFiles, newState, err := layer.GetChangedFiles(".", allFiles, prevState)
			if err != nil {
				fmt.Println("Error:", err)
				return
			}

			prevState = newState
			files = changedFiles

			layer.CreateLayer(".", files)
		}

		if inst.Type == parser.RUN {

			// snapshot BEFORE
			beforeFiles, _ := layer.GetAllFiles(".")
			_, beforeState, _ := layer.GetChangedFiles(".", beforeFiles, prevState)

			// execute RUN
			err := layer.ExecuteRunCommand(inst.Args, ".", state.Env)
			if err != nil {
				fmt.Println("RUN Error:", err)
				return
			}

			// snapshot AFTER
			afterFiles, _ := layer.GetAllFiles(".")
			changedFiles, afterState, _ := layer.GetChangedFiles(".", afterFiles, beforeState)

			prevState = afterState

			fmt.Println("\nRUN changed files:", changedFiles)

			layer.CreateLayer(".", changedFiles)
		}
	}

	layerObj, err := layer.CreateLayer(".", files)
	if err != nil {
		fmt.Println("Layer Error:", err)
		return
	}

	fmt.Println("\nLayer Created:")
	fmt.Println("Digest:", layerObj.Digest)
	fmt.Println("Path:", layerObj.Path)

	fmt.Println("\nBuild State:")
	fmt.Println("WORKDIR:", state.WorkDir)
	fmt.Println("ENV:", state.Env)
	fmt.Println("CMD:", state.Cmd)
}