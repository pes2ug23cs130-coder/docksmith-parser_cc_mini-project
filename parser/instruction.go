package parser

type InstructionType string

const (
	FROM    InstructionType = "FROM"
	COPY    InstructionType = "COPY"
	RUN     InstructionType = "RUN"
	WORKDIR InstructionType = "WORKDIR"
	ENV     InstructionType = "ENV"
	CMD     InstructionType = "CMD"
)

type Instruction struct {
	Type InstructionType
	Args string
	Line int
}