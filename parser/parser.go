package parser

import (
    "bufio"
    "encoding/json"
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
    state := NewBuildState()
    var instructions []Instruction
    lineNo := 0

    for scanner.Scan() {
        lineNo++
        line := strings.TrimSpace(scanner.Text())

        if line == "" || strings.HasPrefix(line, "#") {
            continue
        }

        parts := strings.SplitN(line, " ", 2)
        if len(parts) < 2 {
            return nil, nil, fmt.Errorf("invalid instruction at line %d", lineNo)
        }

        cmd, args := parts[0], strings.TrimSpace(parts[1])

        switch cmd {
        case "FROM":
            instructions = append(instructions, Instruction{FROM, args, lineNo})

        case "COPY":
            instructions = append(instructions, Instruction{COPY, args, lineNo})

        case "RUN":
            instructions = append(instructions, Instruction{RUN, args, lineNo})

        case "WORKDIR":
            state.WorkDir = args
            instructions = append(instructions, Instruction{WORKDIR, args, lineNo})

        case "ENV":
            kv := strings.SplitN(args, "=", 2)
            if len(kv) != 2 {
                return nil, nil, fmt.Errorf("invalid ENV at line %d", lineNo)
            }
            state.Env[kv[0]] = kv[1]
            instructions = append(instructions, Instruction{ENV, args, lineNo})

        case "CMD":
            var cmdArr []string
            if err := json.Unmarshal([]byte(args), &cmdArr); err != nil {
                return nil, nil, fmt.Errorf("CMD must be JSON array at line %d", lineNo)
            }
            state.Cmd = args
            instructions = append(instructions, Instruction{CMD, args, lineNo})

        default:
            return nil, nil, fmt.Errorf("unknown instruction '%s' at line %d", cmd, lineNo)
        }
    }

    return instructions, state, nil
}