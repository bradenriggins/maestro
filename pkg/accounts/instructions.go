package accounts

import "fmt"

// InstructionsContext extends the template context with program info.
type InstructionsContext struct {
	CLAUDEMDContext
	Program string // "claude" or "codex"
}

// GenerateInstructions generates the appropriate instructions file
// (CLAUDE.md or AGENTS.md) based on the program type.
func GenerateInstructions(worktreePath string, ctx InstructionsContext) error {
	if ctx.Program == "" || ctx.Program == "claude" {
		return GenerateCLAUDEMD(worktreePath, ctx.CLAUDEMDContext)
	}
	if ctx.Program == "codex" {
		return GenerateAGENTSMD(worktreePath, ctx.CLAUDEMDContext)
	}
	return fmt.Errorf("unknown program %q: cannot generate instructions file", ctx.Program)
}
