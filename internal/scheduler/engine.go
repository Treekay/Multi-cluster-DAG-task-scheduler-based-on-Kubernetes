package scheduler

import (
	"context"
	"fmt"
	"sort"
)

type Engine struct {
	clusters []clusterState
	executor Executor
}

type clusterState struct {
	cluster Cluster
	used    Resources
}

func NewEngine(clusters []Cluster, executor Executor) *Engine {
	states := make([]clusterState, 0, len(clusters))
	for _, cluster := range clusters {
		states = append(states, clusterState{cluster: cluster})
	}
	return &Engine{clusters: states, executor: executor}
}

func (e *Engine) Run(ctx context.Context, workflow Workflow) (WorkflowResult, error) {
	g, err := buildGraph(workflow)
	if err != nil {
		return WorkflowResult{}, err
	}
	if len(e.clusters) == 0 {
		return WorkflowResult{}, fmt.Errorf("at least one cluster is required")
	}

	statuses := make(map[string]TaskResult, len(workflow.Tasks))
	for _, task := range workflow.Tasks {
		statuses[task.Name] = TaskResult{Name: task.Name, Status: TaskPending}
	}

	ready := g.initialReady()
	completed := 0

	for len(ready) > 0 {
		sort.Strings(ready)
		taskName := ready[0]
		ready = ready[1:]
		task := g.tasks[taskName]

		clusterIndex, err := e.selectCluster(task.Resources)
		if err != nil {
			return WorkflowResult{}, fmt.Errorf("task %q: %w", taskName, err)
		}

		cluster := e.clusters[clusterIndex].cluster
		e.reserve(clusterIndex, task.Resources)
		result := statuses[taskName]
		result.Status = TaskRunning
		result.Cluster = cluster.Name
		statuses[taskName] = result

		if err := e.executor.RunTask(ctx, cluster, task); err != nil {
			e.release(clusterIndex, task.Resources)
			result.Status = TaskFailed
			statuses[taskName] = result
			return WorkflowResult{}, fmt.Errorf("task %q failed on cluster %q: %w", taskName, cluster.Name, err)
		}

		e.release(clusterIndex, task.Resources)
		result.Status = TaskSucceeded
		statuses[taskName] = result
		completed++
		ready = append(ready, g.markCompleted(taskName)...)
	}

	if completed != len(workflow.Tasks) {
		return WorkflowResult{}, fmt.Errorf("workflow stopped before all tasks completed")
	}

	results := make([]TaskResult, 0, len(statuses))
	for _, task := range workflow.Tasks {
		results = append(results, statuses[task.Name])
	}

	return WorkflowResult{WorkflowName: workflow.Name, Tasks: results}, nil
}

func (e *Engine) selectCluster(required Resources) (int, error) {
	return selectCluster(e.clusters, required)
}

func (e *Engine) reserve(clusterIndex int, resources Resources) {
	e.clusters[clusterIndex].used = e.clusters[clusterIndex].used.Add(resources)
}

func (e *Engine) release(clusterIndex int, resources Resources) {
	e.clusters[clusterIndex].used = e.clusters[clusterIndex].used.Sub(resources)
}
