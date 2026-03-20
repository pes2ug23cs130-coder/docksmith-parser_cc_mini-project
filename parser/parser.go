package parser

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func ParseDocksmithfile(path string) ([]Instruction, *BuildState, error) {

	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	var instructions []Instruction
	state := NewBuildState()

	lineNumber := 0

	for scanner.Scan() {

		lineNumber++
		line := strings.TrimSpace(scanner.Text())

		if line == "" {
			continue
		}

		parts := strings.SplitN(line, " ", 2)

		if len(parts) < 2 {
			return nil, nil, fmt.Errorf("invalid instruction at line %d", lineNumber)
		}

		cmd := parts[0]
		args := parts[1]

		switch cmd {

		case "FROM":
			instructions = append(instructions, Instruction{FROM, args, lineNumber})

		case "COPY":
			instructions = append(instructions, Instruction{COPY, args, lineNumber})

		case "RUN":
			instructions = append(instructions, Instruction{RUN, args, lineNumber})

		case "WORKDIR":
			state.WorkDir = args
			instructions = append(instructions, Instruction{WORKDIR, args, lineNumber})

		case "ENV":
			envParts := strings.SplitN(args, "=", 2)
			if len(envParts) != 2 {
				return nil, nil, fmt.Errorf("invalid ENV at line %d", lineNumber)
			}
			state.Env[envParts[0]] = envParts[1]
			instructions = append(instructions, Instruction{ENV, args, lineNumber})

		case "CMD":
			state.Cmd = args
			instructions = append(instructions, Instruction{CMD, args, lineNumber})

		default:
			return nil, nil, fmt.Errorf("unknown instruction '%s' at line %d", cmd, lineNumber)
		}
	}

	return instructions, state, nil
}