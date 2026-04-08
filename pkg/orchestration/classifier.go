package orchestration

import (
	"strings"

	"maestro/pkg/programs"
)

// ClassifierConfidenceThreshold is the minimum score required to confidently
// assign a category. Below this, the classifier defaults to TaskImplementation.
const ClassifierConfidenceThreshold = 3

// ClassificationResult holds the full diagnostic output of the classifier.
type ClassificationResult struct {
	Category  programs.TaskCategory
	Score     int
	Confident bool
}

// categoryPattern pairs a weight with a set of substring terms.
type categoryPattern struct {
	weight int
	terms  []string
}

// categoryRules maps each TaskCategory to its weighted pattern definitions.
var categoryRules = map[programs.TaskCategory][]categoryPattern{
	programs.TaskArchitecture: {
		{weight: 5, terms: []string{"architecture", "system design", "high-level design", "design doc", "architectural decision"}},
		{weight: 4, terms: []string{"service boundary", "microservice", "monolith", "domain model", "bounded context", "event-driven", "clean architecture"}},
		{weight: 3, terms: []string{"scalability", "distributed system", "data flow", "component diagram", "dependency graph"}},
		{weight: 2, terms: []string{"design", "structure", "pattern", "blueprint", "schema"}},
		{weight: 1, terms: []string{"how should", "how do we", "what approach"}},
	},
	programs.TaskDebugging: {
		{weight: 5, terms: []string{"bug", "stacktrace", "stack trace", "panic:", "fatal error", "segfault", "nil pointer", "null pointer", "exception", "traceback"}},
		{weight: 4, terms: []string{"not working", "broken", "fails with", "error:", "crashes", "deadlock", "race condition", "memory leak"}},
		{weight: 3, terms: []string{"debug", "diagnose", "investigate", "root cause", "why is", "why does", "unexpected behavior"}},
		{weight: 2, terms: []string{"fix", "wrong output", "incorrect result"}},
		{weight: 1, terms: []string{"issue", "problem"}},
	},
	programs.TaskImplementation: {
		{weight: 5, terms: []string{"implement", "build", "create", "write", "develop", "add feature", "add support for"}},
		{weight: 4, terms: []string{"endpoint", "api", "handler", "function", "method", "class", "service", "component", "module"}},
		{weight: 3, terms: []string{"feature", "integrate", "wire up", "new", "from scratch"}},
		{weight: 2, terms: []string{"make", "generate", "add"}},
		{weight: 1, terms: []string{"code", "program"}},
	},
	programs.TaskMathLogic: {
		{weight: 5, terms: []string{"algorithm", "proof", "complexity", "big o", "o(n", "compute", "calculate", "mathematical", "formula", "equation", "dynamic programming"}},
		{weight: 4, terms: []string{"time complexity", "space complexity", "sorting", "graph problem", "tree traversal", "shortest path", "knapsack", "matrix multiplication"}},
		{weight: 3, terms: []string{"optimization", "logic", "constraint", "solver", "greedy", "heuristic"}},
		{weight: 2, terms: []string{"efficiently", "performance", "solve"}},
		{weight: 1, terms: []string{"how many"}},
	},
	programs.TaskDocAnalysis: {
		{weight: 5, terms: []string{"summarize", "summarise", "explain this document", "analyze this", "extract", "parse this", "read this file"}},
		{weight: 4, terms: []string{"document", "specification", "rfc", "requirements", "changelog", "release notes", "api spec", "openapi"}},
		{weight: 3, terms: []string{"what does it say", "list the", "find all occurrences", "identify"}},
		{weight: 2, terms: []string{"report", "describe", "explain"}},
		{weight: 1, terms: []string{"file", "text"}},
	},
	programs.TaskSimpleFix: {
		{weight: 5, terms: []string{"typo", "rename", "spelling mistake", "one-liner", "obvious fix", "trivial", "quick fix"}},
		{weight: 4, terms: []string{"variable name", "function name", "whitespace", "formatting", "linting", "indentation", "import order"}},
		{weight: 3, terms: []string{"fix", "update string", "change label", "tweak", "small", "minor"}},
		{weight: 2, terms: []string{"adjust", "correct", "update"}},
		{weight: 1, terms: []string{"just", "only"}},
	},
	programs.TaskCodeReview: {
		{weight: 5, terms: []string{"code review", "review this", "review the code", "review my", "pr review", "pull request review"}},
		{weight: 4, terms: []string{"security review", "audit", "vulnerability", "best practice", "antipattern", "technical debt"}},
		{weight: 3, terms: []string{"improve", "suggest", "recommendation", "assess", "evaluate", "look over"}},
		{weight: 2, terms: []string{"thoughts on", "what do you think", "is this correct"}},
		{weight: 1, terms: []string{"review", "check"}},
	},
	programs.TaskRefactoring: {
		{weight: 5, terms: []string{"refactor", "refactoring", "extract method", "extract function", "extract class", "decompose", "decouple", "restructure"}},
		{weight: 4, terms: []string{"duplication", "dry principle", "reduce duplication", "consolidate", "clean up code", "rewrite"}},
		{weight: 3, terms: []string{"abstraction", "modularity", "maintainability", "simplify", "reorganize", "extract"}},
		{weight: 2, terms: []string{"improve code", "better structure", "cleaner"}},
		{weight: 1, terms: []string{"organize", "rearrange"}},
	},
}

// tieBreakPriority orders categories from highest specificity to lowest.
// When two categories have the same score, the one appearing earlier wins.
var tieBreakPriority = []programs.TaskCategory{
	programs.TaskMathLogic,
	programs.TaskDebugging,
	programs.TaskArchitecture,
	programs.TaskRefactoring,
	programs.TaskCodeReview,
	programs.TaskDocAnalysis,
	programs.TaskSimpleFix,
	programs.TaskImplementation,
}

// InferCategory returns the most likely TaskCategory for the given prompt.
// It is the primary public API for semantic task routing.
func InferCategory(prompt string) programs.TaskCategory {
	return InferCategoryWithResult(prompt).Category
}

// InferCategoryWithResult returns the full ClassificationResult including
// the winning score and whether the classifier is confident in the result.
func InferCategoryWithResult(prompt string) ClassificationResult {
	// Guard: empty or whitespace-only prompt defaults to implementation.
	if strings.TrimSpace(prompt) == "" {
		return ClassificationResult{
			Category:  programs.TaskImplementation,
			Score:     0,
			Confident: false,
		}
	}

	lower := strings.ToLower(prompt)

	// Score each category.
	scores := make(map[programs.TaskCategory]int)
	for cat, patterns := range categoryRules {
		total := 0
		for _, p := range patterns {
			for _, term := range p.terms {
				if strings.Contains(lower, term) {
					total += p.weight
				}
			}
		}
		scores[cat] = total
	}

	// Walk tie-break priority: first category with strict > wins.
	bestCat := tieBreakPriority[0]
	bestScore := scores[bestCat]
	for _, cat := range tieBreakPriority[1:] {
		if scores[cat] > bestScore {
			bestCat = cat
			bestScore = scores[cat]
		}
	}

	// If below threshold, fall back to implementation.
	if bestScore < ClassifierConfidenceThreshold {
		return ClassificationResult{
			Category:  programs.TaskImplementation,
			Score:     bestScore,
			Confident: false,
		}
	}

	return ClassificationResult{
		Category:  bestCat,
		Score:     bestScore,
		Confident: true,
	}
}
