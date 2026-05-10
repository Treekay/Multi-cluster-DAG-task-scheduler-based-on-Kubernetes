package scheduler

import "fmt"

type graph struct {
	tasks        map[string]TaskSpec
	dependents   map[string][]string
	dependencies map[string]map[string]struct{}
}

func buildGraph(workflow Workflow) (*graph, error) {
	if workflow.Name == "" {
		return nil, fmt.Errorf("workflow name is required")
	}
	if len(workflow.Tasks) == 0 {
		return nil, fmt.Errorf("workflow must contain at least one task")
	}

	g := &graph{
		tasks:        make(map[string]TaskSpec, len(workflow.Tasks)),
		dependents:   make(map[string][]string, len(workflow.Tasks)),
		dependencies: make(map[string]map[string]struct{}, len(workflow.Tasks)),
	}

	for _, task := range workflow.Tasks {
		if task.Name == "" {
			return nil, fmt.Errorf("task name is required")
		}
		if _, exists := g.tasks[task.Name]; exists {
			return nil, fmt.Errorf("duplicate task %q", task.Name)
		}
		g.tasks[task.Name] = task
		g.dependencies[task.Name] = make(map[string]struct{}, len(task.DependsOn))
	}

	for _, task := range workflow.Tasks {
		for _, dependency := range task.DependsOn {
			if _, exists := g.tasks[dependency]; !exists {
				return nil, fmt.Errorf("task %q depends on unknown task %q", task.Name, dependency)
			}
			g.dependencies[task.Name][dependency] = struct{}{}
			g.dependents[dependency] = append(g.dependents[dependency], task.Name)
		}
	}

	if err := g.detectCycle(); err != nil {
		return nil, err
	}

	return g, nil
}

func (g *graph) initialReady() []string {
	var ready []string
	for name, dependencies := range g.dependencies {
		if len(dependencies) == 0 {
			ready = append(ready, name)
		}
	}
	return ready
}

func (g *graph) markCompleted(taskName string) []string {
	var ready []string
	for _, dependent := range g.dependents[taskName] {
		delete(g.dependencies[dependent], taskName)
		if len(g.dependencies[dependent]) == 0 {
			ready = append(ready, dependent)
		}
	}
	return ready
}

func (g *graph) detectCycle() error {
	remaining := make(map[string]int, len(g.dependencies))
	for task, dependencies := range g.dependencies {
		remaining[task] = len(dependencies)
	}

	queue := g.initialReady()
	visited := 0
	for len(queue) > 0 {
		task := queue[0]
		queue = queue[1:]
		visited++

		for _, dependent := range g.dependents[task] {
			remaining[dependent]--
			if remaining[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}

	if visited != len(g.tasks) {
		return fmt.Errorf("workflow contains a dependency cycle")
	}
	return nil
}
