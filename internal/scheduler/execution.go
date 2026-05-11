package scheduler

import (
	"context"
	"fmt"
	"sort"
)

type ExecutionRequest struct {
	Workflow Workflow  `json:"workflow"`
	Clusters []Cluster `json:"clusters"`
}

type ExecutionResult struct {
	Workflow Workflow         `json:"workflow"`
	Clusters []Cluster        `json:"clusters"`
	Steps    []SimulationStep `json:"steps"`
	Error    string           `json:"error,omitempty"`
}

func ExecuteWorkflow(ctx context.Context, workflow Workflow, clusters []Cluster, executor Executor) (ExecutionResult, error) {
	g, err := buildGraph(workflow)
	if err != nil {
		return ExecutionResult{}, err
	}
	if len(clusters) == 0 {
		return ExecutionResult{}, fmt.Errorf("at least one cluster is required")
	}

	states := make([]clusterState, 0, len(clusters))
	for _, cluster := range clusters {
		states = append(states, clusterState{cluster: cluster})
	}

	taskViews := make(map[string]TaskView, len(workflow.Tasks))
	for _, task := range workflow.Tasks {
		taskViews[task.Name] = TaskView{
			Name:      task.Name,
			Status:    TaskPending,
			DependsOn: append([]string(nil), task.DependsOn...),
			CPU:       task.Resources.CPU,
			MemoryMiB: task.Resources.MemoryMiB,
		}
	}

	ready := g.initialReady()
	sort.Strings(ready)
	steps := []SimulationStep{buildSimulationStep(0, "init", "Workflow accepted and Kubernetes execution started.", "", "", workflow, taskViews, states, ready)}
	for _, taskName := range ready {
		view := taskViews[taskName]
		view.Status = TaskReady
		taskViews[taskName] = view
		steps = append(steps, buildSimulationStep(len(steps), "ready", fmt.Sprintf("%s is ready to run.", taskName), taskName, "", workflow, taskViews, states, ready))
	}

	completed := 0
	for len(ready) > 0 {
		sort.Strings(ready)
		taskName := ready[0]
		ready = ready[1:]
		task := g.tasks[taskName]

		clusterIndex, err := selectCluster(states, task.Resources)
		if err != nil {
			return executionFailure(workflow, clusters, steps, err)
		}

		states[clusterIndex].used = states[clusterIndex].used.Add(task.Resources)
		cluster := states[clusterIndex].cluster
		view := taskViews[taskName]
		view.Status = TaskRunning
		view.Cluster = cluster.Name
		taskViews[taskName] = view
		steps = append(steps, buildSimulationStep(len(steps), "scheduled", fmt.Sprintf("%s submitted to %s.", taskName, cluster.Name), taskName, cluster.Name, workflow, taskViews, states, ready))

		if err := executor.RunTask(ctx, cluster, task); err != nil {
			states[clusterIndex].used = states[clusterIndex].used.Sub(task.Resources)
			view.Status = TaskFailed
			taskViews[taskName] = view
			steps = append(steps, buildSimulationStep(len(steps), "failed", fmt.Sprintf("%s failed on %s: %v", taskName, cluster.Name, err), taskName, cluster.Name, workflow, taskViews, states, ready))
			return executionFailure(workflow, clusters, steps, err)
		}

		states[clusterIndex].used = states[clusterIndex].used.Sub(task.Resources)
		view.Status = TaskSucceeded
		taskViews[taskName] = view
		completed++
		steps = append(steps, buildSimulationStep(len(steps), "succeeded", fmt.Sprintf("%s completed on %s.", taskName, cluster.Name), taskName, cluster.Name, workflow, taskViews, states, ready))

		newReady := g.markCompleted(taskName)
		sort.Strings(newReady)
		ready = append(ready, newReady...)
		sort.Strings(ready)
		for _, readyTask := range newReady {
			view := taskViews[readyTask]
			view.Status = TaskReady
			taskViews[readyTask] = view
			steps = append(steps, buildSimulationStep(len(steps), "ready", fmt.Sprintf("%s is ready to run.", readyTask), readyTask, "", workflow, taskViews, states, ready))
		}
	}

	if completed != len(workflow.Tasks) {
		err := fmt.Errorf("workflow stopped before all tasks completed")
		return executionFailure(workflow, clusters, steps, err)
	}

	steps = append(steps, buildSimulationStep(len(steps), "finished", "Kubernetes workflow execution finished.", "", "", workflow, taskViews, states, nil))
	return ExecutionResult{Workflow: workflow, Clusters: clusters, Steps: steps}, nil
}

func executionFailure(workflow Workflow, clusters []Cluster, steps []SimulationStep, err error) (ExecutionResult, error) {
	return ExecutionResult{
		Workflow: workflow,
		Clusters: clusters,
		Steps:    steps,
		Error:    err.Error(),
	}, err
}
