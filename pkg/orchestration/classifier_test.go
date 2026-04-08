package orchestration

import (
	"testing"

	"maestro/pkg/programs"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInferCategory(t *testing.T) {
	tests := []struct {
		name     string
		prompt   string
		expected programs.TaskCategory
	}{
		{
			name:     "architecture prompt",
			prompt:   "Design the overall architecture for a multi-tenant SaaS billing system",
			expected: programs.TaskArchitecture,
		},
		{
			name:     "debugging prompt",
			prompt:   "The server is panicking with a nil pointer dereference",
			expected: programs.TaskDebugging,
		},
		{
			name:     "implementation prompt",
			prompt:   "Implement a new REST endpoint POST /api/v2/users",
			expected: programs.TaskImplementation,
		},
		{
			name:     "simple fix prompt",
			prompt:   "Fix the typo in the error message — recieve should be receive",
			expected: programs.TaskSimpleFix,
		},
		{
			name:     "code review prompt",
			prompt:   "Review this authentication middleware for security issues",
			expected: programs.TaskCodeReview,
		},
		{
			name:     "refactoring prompt",
			prompt:   "Refactor the payment module to extract a separate billing service",
			expected: programs.TaskRefactoring,
		},
		{
			name:     "math/logic prompt",
			prompt:   "What is the time complexity of this recursive tree traversal algorithm?",
			expected: programs.TaskMathLogic,
		},
		{
			name:     "doc analysis prompt",
			prompt:   "Summarize the API specification document",
			expected: programs.TaskDocAnalysis,
		},
		{
			name:     "empty prompt guard",
			prompt:   "",
			expected: programs.TaskImplementation,
		},
		{
			name:     "below threshold",
			prompt:   "hi",
			expected: programs.TaskImplementation,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InferCategory(tt.prompt)
			assert.Equal(t, tt.expected, got, "InferCategory(%q)", tt.prompt)
		})
	}
}

func TestInferCategory_Determinism(t *testing.T) {
	prompt := "Design the overall architecture for a multi-tenant SaaS billing system"
	first := InferCategory(prompt)
	for i := 0; i < 10; i++ {
		got := InferCategory(prompt)
		assert.Equal(t, first, got, "run %d: expected deterministic result", i)
	}
}

func TestInferCategoryWithResult(t *testing.T) {
	t.Run("confident result has positive score", func(t *testing.T) {
		result := InferCategoryWithResult("Implement a new REST endpoint POST /api/v2/users")
		assert.Equal(t, programs.TaskImplementation, result.Category)
		assert.GreaterOrEqual(t, result.Score, ClassifierConfidenceThreshold)
		assert.True(t, result.Confident)
	})

	t.Run("below threshold is not confident", func(t *testing.T) {
		result := InferCategoryWithResult("hi")
		assert.Equal(t, programs.TaskImplementation, result.Category)
		assert.Less(t, result.Score, ClassifierConfidenceThreshold)
		assert.False(t, result.Confident)
	})

	t.Run("empty prompt returns zero score", func(t *testing.T) {
		result := InferCategoryWithResult("")
		assert.Equal(t, programs.TaskImplementation, result.Category)
		assert.Equal(t, 0, result.Score)
		assert.False(t, result.Confident)
	})

	t.Run("whitespace-only prompt treated as empty", func(t *testing.T) {
		result := InferCategoryWithResult("   \t\n  ")
		assert.Equal(t, programs.TaskImplementation, result.Category)
		assert.Equal(t, 0, result.Score)
		assert.False(t, result.Confident)
	})

	t.Run("debugging prompt is confident", func(t *testing.T) {
		result := InferCategoryWithResult("The server is panicking with a nil pointer dereference")
		assert.Equal(t, programs.TaskDebugging, result.Category)
		require.True(t, result.Confident)
		assert.GreaterOrEqual(t, result.Score, ClassifierConfidenceThreshold)
	})
}
