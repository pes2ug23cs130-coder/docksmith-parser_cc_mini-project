package parser

type BuildState struct {
	WorkDir string
	Env     map[string]string
	Cmd     string
}

func NewBuildState() *BuildState {
	return &BuildState{
		WorkDir: "",
		Env:     make(map[string]string),
		Cmd:     "",
	}
}