package orchestration

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// PipelineTask defines a single task within a pipeline YAML file.
type PipelineTask struct {
	ID        string   `yaml:"id"`
	Worker    string   `yaml:"worker"`
	DependsOn []string `yaml:"depends_on"`
	Prompt    string   `yaml:"prompt"`
}

// Pipeline is the top-level structure of a pipeline YAML file.
type Pipeline struct {
	Name  string         `yaml:"name"`
	Tasks []PipelineTask `yaml:"tasks"`
}

// PipelineResult holds the outcome of running a pipeline.
type PipelineResult struct {
	PipelineName string
	TaskIDs      map[string]string // logical ID -> real task ID
	Dispatched   []string          // real task IDs that were immediately dispatched (zero-dep) and need tmux delivery
	Errors       []string
}

// LoadPipeline reads and validates a pipeline YAML file.
func LoadPipeline(path string) (*Pipeline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read pipeline file: %w", err)
	}

	var p Pipeline
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("failed to parse pipeline YAML: %w", err)
	}

	if p.Name == "" {
		return nil, fmt.Errorf("pipeline must have a name")
	}
	if len(p.Tasks) == 0 {
		return nil, fmt.Errorf("pipeline must have at least one task")
	}

	// Validate fields
	ids := make(map[string]bool)
	for _, t := range p.Tasks {
		if t.ID == "" {
			return nil, fmt.Errorf("pipeline task missing id")
		}
		if t.Worker == "" {
			return nil, fmt.Errorf("pipeline task %q missing worker", t.ID)
		}
		if t.Prompt == "" {
			return nil, fmt.Errorf("pipeline task %q missing prompt", t.ID)
		}
		if ids[t.ID] {
			return nil, fmt.Errorf("duplicate pipeline task id %q", t.ID)
		}
		ids[t.ID] = true
	}

	// Validate depends_on references
	for _, t := range p.Tasks {
		seen := make(map[string]bool)
		for _, dep := range t.DependsOn {
			if dep == t.ID {
				return nil, fmt.Errorf("pipeline task %q depends on itself", t.ID)
			}
			if !ids[dep] {
				return nil, fmt.Errorf("pipeline task %q depends on unknown task %q", t.ID, dep)
			}
			if seen[dep] {
				return nil, fmt.Errorf("pipeline task %q has duplicate dependency %q", t.ID, dep)
			}
			seen[dep] = true
		}
	}

	return &p, nil
}

// RunPipeline creates all tasks defined in a pipeline in topological order.
// Tasks with no dependencies are dispatched immediately (StatusDispatched);
// tasks with dependencies start as StatusPending.
func RunPipeline(p *Pipeline, store *TaskStore) (*PipelineResult, error) {
	result := &PipelineResult{
		PipelineName: p.Name,
		TaskIDs:      make(map[string]string),
	}

	// Check for cycles in the pipeline graph
	if err := detectPipelineCycle(p); err != nil {
		return nil, err
	}

	// Topological sort to determine creation order
	order, err := topologicalSort(p)
	if err != nil {
		return nil, err
	}

	// Create tasks in topological order
	for _, logicalID := range order {
		var pt *PipelineTask
		for i := range p.Tasks {
			if p.Tasks[i].ID == logicalID {
				pt = &p.Tasks[i]
				break
			}
		}

		taskID, err := GenerateTaskID()
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to generate ID for %q: %v", logicalID, err))
			continue
		}

		// Translate logical dep IDs to real task IDs
		var realDeps []string
		depMissing := false
		for _, dep := range pt.DependsOn {
			realID, ok := result.TaskIDs[dep]
			if !ok {
				result.Errors = append(result.Errors, fmt.Sprintf("dependency %q for task %q not yet created — skipping task", dep, logicalID))
				depMissing = true
				break
			}
			realDeps = append(realDeps, realID)
		}
		if depMissing {
			continue
		}

		var opts *CreateOptions
		if len(realDeps) > 0 {
			opts = &CreateOptions{
				DependsOn:  realDeps,
				PipelineID: p.Name,
			}
		} else {
			opts = &CreateOptions{
				PipelineID: p.Name,
			}
		}

		statusPath := store.StatusFilePath(pt.Worker)
		task, err := store.Create(taskID, pt.Prompt, pt.Worker, "", statusPath, "pipeline", opts)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to create task %q: %v", logicalID, err))
			continue
		}

		result.TaskIDs[logicalID] = taskID

		// Track zero-dep tasks that were immediately dispatched so the caller
		// can deliver them to their workers' tmux sessions.
		if task.Status == StatusDispatched {
			result.Dispatched = append(result.Dispatched, taskID)
		}
	}

	return result, nil
}

// detectPipelineCycle checks for cycles in the pipeline's dependency graph.
func detectPipelineCycle(p *Pipeline) error {
	// Build adjacency list
	adj := make(map[string][]string)
	for _, t := range p.Tasks {
		adj[t.ID] = t.DependsOn
	}

	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := make(map[string]int)
	for _, t := range p.Tasks {
		color[t.ID] = white
	}

	var dfs func(node string) error
	dfs = func(node string) error {
		color[node] = gray
		for _, dep := range adj[node] {
			if color[dep] == gray {
				return fmt.Errorf("pipeline has a dependency cycle involving %q and %q", node, dep)
			}
			if color[dep] == white {
				if err := dfs(dep); err != nil {
					return err
				}
			}
		}
		color[node] = black
		return nil
	}

	for _, t := range p.Tasks {
		if color[t.ID] == white {
			if err := dfs(t.ID); err != nil {
				return err
			}
		}
	}

	return nil
}

// topologicalSort returns task IDs in topological order (dependencies first).
func topologicalSort(p *Pipeline) ([]string, error) {
	adj := make(map[string][]string)
	inDegree := make(map[string]int)
	for _, t := range p.Tasks {
		if _, exists := inDegree[t.ID]; !exists {
			inDegree[t.ID] = 0
		}
		adj[t.ID] = t.DependsOn
	}

	// In our graph, task -> depends_on means task depends on dep.
	// For topological sort, we need reverse: dep is a prerequisite of task.
	// inDegree: count how many prerequisites a task has.
	reverse := make(map[string][]string)
	for _, t := range p.Tasks {
		for _, dep := range t.DependsOn {
			reverse[dep] = append(reverse[dep], t.ID)
			inDegree[t.ID]++
		}
	}

	// Kahn's algorithm
	var queue []string
	for _, t := range p.Tasks {
		if inDegree[t.ID] == 0 {
			queue = append(queue, t.ID)
		}
	}

	var order []string
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		order = append(order, node)

		for _, dependent := range reverse[node] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}

	if len(order) != len(p.Tasks) {
		return nil, fmt.Errorf("pipeline has a dependency cycle (could not complete topological sort)")
	}

	return order, nil
}
