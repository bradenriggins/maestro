package programs

import (
	"sort"
	"strings"
)

// TaskCategory represents a type of programming task for model selection.
type TaskCategory string

const (
	TaskArchitecture   TaskCategory = "architecture"
	TaskImplementation TaskCategory = "implementation"
	TaskDebugging      TaskCategory = "debugging"
	TaskMathLogic      TaskCategory = "math_logic"
	TaskDocAnalysis    TaskCategory = "doc_analysis"
	TaskSimpleFix      TaskCategory = "simple_fix"
	TaskCodeReview     TaskCategory = "code_review"
	TaskRefactoring    TaskCategory = "refactoring"
)

// AllTaskCategories returns all defined task categories.
var AllTaskCategories = []TaskCategory{
	TaskArchitecture, TaskImplementation, TaskDebugging,
	TaskMathLogic, TaskDocAnalysis, TaskSimpleFix,
	TaskCodeReview, TaskRefactoring,
}

// ModelProfile describes a model's capabilities and performance characteristics.
type ModelProfile struct {
	ID          string
	DisplayName string
	Program     string
	Strengths   map[TaskCategory]int // 1-10 score per category
	SpeedTier   string               // "slow", "medium", "fast"
}

// ModelProfiles is the static registry of all known model profiles.
var ModelProfiles = map[string]ModelProfile{
	"opus-4.6": {
		ID: "opus-4.6", DisplayName: "Claude Opus 4.6", Program: "claude",
		Strengths: map[TaskCategory]int{
			TaskArchitecture: 10, TaskImplementation: 8, TaskDebugging: 9,
			TaskMathLogic: 9, TaskDocAnalysis: 9, TaskSimpleFix: 7,
			TaskCodeReview: 10, TaskRefactoring: 9,
		},
		SpeedTier: "slow",
	},
	"sonnet-4.6": {
		ID: "sonnet-4.6", DisplayName: "Claude Sonnet 4.6", Program: "claude",
		Strengths: map[TaskCategory]int{
			TaskArchitecture: 7, TaskImplementation: 10, TaskDebugging: 8,
			TaskMathLogic: 6, TaskDocAnalysis: 7, TaskSimpleFix: 9,
			TaskCodeReview: 8, TaskRefactoring: 9,
		},
		SpeedTier: "fast",
	},
	"haiku-4.5": {
		ID: "haiku-4.5", DisplayName: "Claude Haiku 4.5", Program: "claude",
		Strengths: map[TaskCategory]int{
			TaskArchitecture: 5, TaskImplementation: 7, TaskDebugging: 6,
			TaskMathLogic: 5, TaskDocAnalysis: 5, TaskSimpleFix: 10,
			TaskCodeReview: 6, TaskRefactoring: 6,
		},
		SpeedTier: "fast",
	},
	"gpt-5.4-thinking": {
		ID: "gpt-5.4-thinking", DisplayName: "GPT-5.4 Thinking", Program: "codex",
		Strengths: map[TaskCategory]int{
			TaskArchitecture: 10, TaskImplementation: 8, TaskDebugging: 9,
			TaskMathLogic: 10, TaskDocAnalysis: 8, TaskSimpleFix: 6,
			TaskCodeReview: 9, TaskRefactoring: 8,
		},
		SpeedTier: "slow",
	},
	"gpt-5.4-mini": {
		ID: "gpt-5.4-mini", DisplayName: "GPT-5.4 mini", Program: "codex",
		Strengths: map[TaskCategory]int{
			TaskArchitecture: 6, TaskImplementation: 8, TaskDebugging: 7,
			TaskMathLogic: 7, TaskDocAnalysis: 6, TaskSimpleFix: 9,
			TaskCodeReview: 7, TaskRefactoring: 7,
		},
		SpeedTier: "fast",
	},
	"gpt-5.4-nano": {
		ID: "gpt-5.4-nano", DisplayName: "GPT-5.4 nano", Program: "codex",
		Strengths: map[TaskCategory]int{
			TaskArchitecture: 4, TaskImplementation: 6, TaskDebugging: 5,
			TaskMathLogic: 5, TaskDocAnalysis: 4, TaskSimpleFix: 9,
			TaskCodeReview: 5, TaskRefactoring: 5,
		},
		SpeedTier: "fast",
	},
	"gpt-5.3-codex": {
		ID: "gpt-5.3-codex", DisplayName: "GPT-5.3-Codex", Program: "codex",
		Strengths: map[TaskCategory]int{
			TaskArchitecture: 8, TaskImplementation: 10, TaskDebugging: 9,
			TaskMathLogic: 7, TaskDocAnalysis: 7, TaskSimpleFix: 8,
			TaskCodeReview: 8, TaskRefactoring: 9,
		},
		SpeedTier: "medium",
	},
	"gpt-5.1-codex-max": {
		ID: "gpt-5.1-codex-max", DisplayName: "GPT-5.1-Codex-Max", Program: "codex",
		Strengths: map[TaskCategory]int{
			TaskArchitecture: 9, TaskImplementation: 9, TaskDebugging: 8,
			TaskMathLogic: 8, TaskDocAnalysis: 8, TaskSimpleFix: 7,
			TaskCodeReview: 9, TaskRefactoring: 8,
		},
		SpeedTier: "slow",
	},
	"gpt-5-codex-mini": {
		ID: "gpt-5-codex-mini", DisplayName: "GPT-5-Codex-Mini", Program: "codex",
		Strengths: map[TaskCategory]int{
			TaskArchitecture: 5, TaskImplementation: 8, TaskDebugging: 6,
			TaskMathLogic: 6, TaskDocAnalysis: 5, TaskSimpleFix: 9,
			TaskCodeReview: 6, TaskRefactoring: 7,
		},
		SpeedTier: "fast",
	},
}

// GetModelProfile returns the profile for a model ID.
func GetModelProfile(id string) (ModelProfile, bool) {
	p, ok := ModelProfiles[id]
	return p, ok
}

// ModelsForProgram returns all model profiles for the given program.
func ModelsForProgram(program string) []ModelProfile {
	var result []ModelProfile
	for _, p := range ModelProfiles {
		if p.Program == program {
			result = append(result, p)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

// BestModelForTask returns the model ID with the highest strength score
// for the given task category, from the list of available model IDs.
// Ties are broken alphabetically by model ID for determinism.
// Returns empty string if no models match.
func BestModelForTask(category TaskCategory, availableModelIDs []string) string {
	bestID := ""
	bestScore := 0
	for _, id := range availableModelIDs {
		profile, ok := ModelProfiles[id]
		if !ok {
			continue
		}
		score := profile.Strengths[category]
		if score > bestScore || (score == bestScore && bestID != "" && id < bestID) {
			bestScore = score
			bestID = id
		}
	}
	return bestID
}

// FormatModelStrengths returns a human-readable summary of a model's top strengths.
func FormatModelStrengths(modelID string) string {
	profile, ok := ModelProfiles[modelID]
	if !ok {
		return "unknown"
	}
	// Find top 3 strengths
	type scored struct {
		category TaskCategory
		score    int
	}
	var scores []scored
	for cat, s := range profile.Strengths {
		scores = append(scores, scored{cat, s})
	}
	sort.Slice(scores, func(i, j int) bool {
		if scores[i].score != scores[j].score {
			return scores[i].score > scores[j].score
		}
		return string(scores[i].category) < string(scores[j].category)
	})
	var parts []string
	limit := 3
	if len(scores) < limit {
		limit = len(scores)
	}
	for i := 0; i < limit; i++ {
		parts = append(parts, string(scores[i].category))
	}
	return strings.Join(parts, ", ")
}
