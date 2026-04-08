package programs

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// programs.go tests
// ---------------------------------------------------------------------------

func TestGet_Claude(t *testing.T) {
	spec, ok := Get("claude")
	if !ok {
		t.Fatal("Get(\"claude\") returned ok=false, want ok=true")
	}
	if spec.Name == "" {
		t.Error("Get(\"claude\") spec.Name is empty")
	}
	if spec.Binary == "" {
		t.Error("Get(\"claude\") spec.Binary is empty")
	}
	if spec.ConfigDirEnvVar == "" {
		t.Error("Get(\"claude\") spec.ConfigDirEnvVar is empty")
	}
	if spec.InstructionsFile == "" {
		t.Error("Get(\"claude\") spec.InstructionsFile is empty")
	}
}

func TestGet_Codex(t *testing.T) {
	spec, ok := Get("codex")
	if !ok {
		t.Fatal("Get(\"codex\") returned ok=false, want ok=true")
	}
	if spec.Name == "" {
		t.Error("Get(\"codex\") spec.Name is empty")
	}
	if spec.Binary == "" {
		t.Error("Get(\"codex\") spec.Binary is empty")
	}
	if spec.ConfigDirEnvVar == "" {
		t.Error("Get(\"codex\") spec.ConfigDirEnvVar is empty")
	}
	if spec.InstructionsFile == "" {
		t.Error("Get(\"codex\") spec.InstructionsFile is empty")
	}
}

// Get("") is documented to redirect to "claude", so ok must be true.
func TestGet_EmptyString(t *testing.T) {
	spec, ok := Get("")
	if !ok {
		t.Fatal("Get(\"\") returned ok=false, want ok=true (should default to claude)")
	}
	if spec.Name != "claude" {
		t.Errorf("Get(\"\") spec.Name = %q, want \"claude\"", spec.Name)
	}
}

// Get("unknown") should return ok=false and a zero-value spec.
func TestGet_Unknown(t *testing.T) {
	spec, ok := Get("unknown")
	if ok {
		t.Error("Get(\"unknown\") returned ok=true, want ok=false")
	}
	if spec.Name != "" {
		t.Errorf("Get(\"unknown\") spec.Name = %q, want empty string", spec.Name)
	}
}

func TestValidProgram_Claude(t *testing.T) {
	if !ValidProgram("claude") {
		t.Error("ValidProgram(\"claude\") = false, want true")
	}
}

func TestValidProgram_Codex(t *testing.T) {
	if !ValidProgram("codex") {
		t.Error("ValidProgram(\"codex\") = false, want true")
	}
}

func TestValidProgram_Empty(t *testing.T) {
	if ValidProgram("") {
		t.Error("ValidProgram(\"\") = true, want false")
	}
}

func TestValidProgram_Unknown(t *testing.T) {
	if ValidProgram("unknown") {
		t.Error("ValidProgram(\"unknown\") = true, want false")
	}
}

func TestNames_ContainsBothPrograms(t *testing.T) {
	names := Names()
	found := make(map[string]bool)
	for _, n := range names {
		found[n] = true
	}
	if !found["claude"] {
		t.Error("Names() does not contain \"claude\"")
	}
	if !found["codex"] {
		t.Error("Names() does not contain \"codex\"")
	}
}

func TestNames_Sorted(t *testing.T) {
	names := Names()
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Errorf("Names() is not sorted: %q comes after %q", names[i], names[i-1])
		}
	}
}

func TestDefaultSpec_IsClaude(t *testing.T) {
	spec := DefaultSpec()
	if spec.Name != "claude" {
		t.Errorf("DefaultSpec().Name = %q, want \"claude\"", spec.Name)
	}
	if spec.Binary != "claude" {
		t.Errorf("DefaultSpec().Binary = %q, want \"claude\"", spec.Binary)
	}
	if spec.ConfigDirEnvVar != "CLAUDE_CONFIG_DIR" {
		t.Errorf("DefaultSpec().ConfigDirEnvVar = %q, want \"CLAUDE_CONFIG_DIR\"", spec.ConfigDirEnvVar)
	}
	if spec.InstructionsFile != "CLAUDE.md" {
		t.Errorf("DefaultSpec().InstructionsFile = %q, want \"CLAUDE.md\"", spec.InstructionsFile)
	}
}

// ---------------------------------------------------------------------------
// models.go tests
// ---------------------------------------------------------------------------

func TestGetModelProfile_Sonnet(t *testing.T) {
	p, ok := GetModelProfile("sonnet-4.6")
	if !ok {
		t.Fatal("GetModelProfile(\"sonnet-4.6\") returned ok=false")
	}
	if p.Program != "claude" {
		t.Errorf("GetModelProfile(\"sonnet-4.6\").Program = %q, want \"claude\"", p.Program)
	}
	if len(p.Strengths) == 0 {
		t.Error("GetModelProfile(\"sonnet-4.6\").Strengths is empty, want non-zero strengths")
	}
	for cat, score := range p.Strengths {
		if score == 0 {
			t.Errorf("GetModelProfile(\"sonnet-4.6\").Strengths[%q] = 0, want > 0", cat)
		}
	}
}

func TestGetModelProfile_GptCodex(t *testing.T) {
	p, ok := GetModelProfile("gpt-5.3-codex")
	if !ok {
		t.Fatal("GetModelProfile(\"gpt-5.3-codex\") returned ok=false")
	}
	if p.Program != "codex" {
		t.Errorf("GetModelProfile(\"gpt-5.3-codex\").Program = %q, want \"codex\"", p.Program)
	}
}

func TestGetModelProfile_Empty(t *testing.T) {
	_, ok := GetModelProfile("")
	if ok {
		t.Error("GetModelProfile(\"\") returned ok=true, want ok=false")
	}
}

func TestGetModelProfile_Nonexistent(t *testing.T) {
	_, ok := GetModelProfile("nonexistent")
	if ok {
		t.Error("GetModelProfile(\"nonexistent\") returned ok=true, want ok=false")
	}
}

func TestBestModelForTask_ImplementationClaude(t *testing.T) {
	claudeModels := []string{"opus-4.6", "sonnet-4.6", "haiku-4.5"}
	best := BestModelForTask(TaskImplementation, claudeModels)
	if best == "" {
		t.Error("BestModelForTask(TaskImplementation, claude models) returned empty string")
	}
	// Verify the returned model is actually a claude model
	p, ok := GetModelProfile(best)
	if !ok {
		t.Errorf("BestModelForTask returned unknown model %q", best)
	}
	if p.Program != "claude" {
		t.Errorf("BestModelForTask(TaskImplementation, claude models) returned a %q model %q, want claude", p.Program, best)
	}
}

func TestBestModelForTask_ImplementationCodex(t *testing.T) {
	codexModels := []string{"gpt-5.4-thinking", "gpt-5.4-mini", "gpt-5.4-nano", "gpt-5.3-codex", "gpt-5.1-codex-max", "gpt-5-codex-mini"}
	best := BestModelForTask(TaskImplementation, codexModels)
	if best == "" {
		t.Error("BestModelForTask(TaskImplementation, codex models) returned empty string")
	}
	p, ok := GetModelProfile(best)
	if !ok {
		t.Errorf("BestModelForTask returned unknown model %q", best)
	}
	if p.Program != "codex" {
		t.Errorf("BestModelForTask(TaskImplementation, codex models) returned a %q model %q, want codex", p.Program, best)
	}
}

// gpt-5.4-thinking has math_logic=10, the highest among codex models.
func TestBestModelForTask_MathLogicCodex(t *testing.T) {
	codexModels := []string{"gpt-5.4-thinking", "gpt-5.4-mini", "gpt-5.4-nano", "gpt-5.3-codex", "gpt-5.1-codex-max", "gpt-5-codex-mini"}
	best := BestModelForTask(TaskMathLogic, codexModels)
	if best == "" {
		t.Error("BestModelForTask(TaskMathLogic, codex models) returned empty string")
	}
	p, ok := GetModelProfile(best)
	if !ok {
		t.Fatalf("BestModelForTask returned unknown model %q", best)
	}
	// Confirm the winner has the highest math_logic score among all codex models
	bestScore := p.Strengths[TaskMathLogic]
	for _, id := range codexModels {
		prof, exists := GetModelProfile(id)
		if !exists {
			continue
		}
		if s := prof.Strengths[TaskMathLogic]; s > bestScore {
			t.Errorf("BestModelForTask chose %q (score %d) but %q has higher score %d", best, bestScore, id, s)
		}
	}
}

func TestBestModelForTask_UnknownProgram(t *testing.T) {
	// No models for an unknown program — pass an empty slice.
	best := BestModelForTask(TaskImplementation, []string{})
	if best != "" {
		t.Errorf("BestModelForTask(TaskImplementation, []) = %q, want empty string", best)
	}
}

// BestModelForTask should not panic when given an empty category string.
func TestBestModelForTask_EmptyCategory(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("BestModelForTask panicked with empty category: %v", r)
		}
	}()
	claudeModels := []string{"opus-4.6", "sonnet-4.6", "haiku-4.5"}
	// Empty category maps to score 0 for every model (missing key in map returns zero).
	// Should return a non-empty model chosen by tie-break, or empty — no panic is required.
	_ = BestModelForTask("", claudeModels)
}

// BestModelForTask with unknown model IDs should gracefully skip them.
func TestBestModelForTask_UnknownModelIDs(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("BestModelForTask panicked with unknown model IDs: %v", r)
		}
	}()
	best := BestModelForTask(TaskImplementation, []string{"does-not-exist", "also-unknown"})
	if best != "" {
		t.Errorf("BestModelForTask with all-unknown IDs = %q, want empty string", best)
	}
}

func TestModelsForProgram_Claude(t *testing.T) {
	models := ModelsForProgram("claude")
	if len(models) == 0 {
		t.Fatal("ModelsForProgram(\"claude\") returned empty slice")
	}
	for _, m := range models {
		if m.Program != "claude" {
			t.Errorf("ModelsForProgram(\"claude\") returned model with Program=%q, want \"claude\"", m.Program)
		}
	}
	// Verify at least the three known claude models are present
	ids := make(map[string]bool)
	for _, m := range models {
		ids[m.ID] = true
	}
	for _, expected := range []string{"opus-4.6", "sonnet-4.6", "haiku-4.5"} {
		if !ids[expected] {
			t.Errorf("ModelsForProgram(\"claude\") missing expected model %q", expected)
		}
	}
}

func TestModelsForProgram_Codex(t *testing.T) {
	models := ModelsForProgram("codex")
	if len(models) == 0 {
		t.Fatal("ModelsForProgram(\"codex\") returned empty slice")
	}
	for _, m := range models {
		if m.Program != "codex" {
			t.Errorf("ModelsForProgram(\"codex\") returned model with Program=%q, want \"codex\"", m.Program)
		}
	}
}

// ModelsForProgram("unknown") must not panic and must return a safe (nil/empty) slice.
func TestModelsForProgram_Unknown(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("ModelsForProgram(\"unknown\") panicked: %v", r)
		}
	}()
	models := ModelsForProgram("unknown")
	if len(models) != 0 {
		t.Errorf("ModelsForProgram(\"unknown\") len = %d, want 0", len(models))
	}
}

func TestFormatModelStrengths_Sonnet(t *testing.T) {
	result := FormatModelStrengths("sonnet-4.6")
	if result == "" {
		t.Error("FormatModelStrengths(\"sonnet-4.6\") returned empty string, want non-empty")
	}
	// Must contain at least one known task category name
	hasCategory := false
	for _, cat := range AllTaskCategories {
		if strings.Contains(result, string(cat)) {
			hasCategory = true
			break
		}
	}
	if !hasCategory {
		t.Errorf("FormatModelStrengths(\"sonnet-4.6\") = %q, does not contain any task category name", result)
	}
}

// FormatModelStrengths for a nonexistent model returns "unknown", not empty and not a panic.
func TestFormatModelStrengths_Nonexistent(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("FormatModelStrengths(\"nonexistent\") panicked: %v", r)
		}
	}()
	result := FormatModelStrengths("nonexistent")
	// The implementation returns "unknown" for missing models
	if result != "unknown" {
		t.Errorf("FormatModelStrengths(\"nonexistent\") = %q, want \"unknown\"", result)
	}
}

// BestModelForTask must be deterministic — same inputs always produce same output.
func TestBestModelForTask_Determinism(t *testing.T) {
	claudeModels := []string{"opus-4.6", "sonnet-4.6", "haiku-4.5"}
	first := BestModelForTask(TaskImplementation, claudeModels)
	for i := 0; i < 9; i++ {
		got := BestModelForTask(TaskImplementation, claudeModels)
		if got != first {
			t.Errorf("BestModelForTask is non-deterministic: iteration %d returned %q, first call returned %q", i+1, got, first)
		}
	}
}
