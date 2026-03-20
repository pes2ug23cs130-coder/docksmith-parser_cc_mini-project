package main

import (
	"docksmith/parser"
	"fmt"
)

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

	fmt.Println("\nBuild State:")
	fmt.Println("WORKDIR:", state.WorkDir)
	fmt.Println("ENV:", state.Env)
	fmt.Println("CMD:", state.Cmd)
}