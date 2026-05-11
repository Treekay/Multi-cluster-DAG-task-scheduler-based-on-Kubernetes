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
	workflow = NormalizeWorkflowCosts(workflow)
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

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	completed := 0
	running := 0
	results := make(chan taskExecutionResult, len(workflow.Tasks))

	for completed < len(workflow.Tasks) {
		ready = sortReadyByCost(ready, g.tasks)
		scheduled := false
		blocked := ready[:0]
		for _, taskName := range ready {
			task := g.tasks[taskName]
			clusterIndex, err := selectCluster(states, task.Resources)
			if err != nil {
				blocked = append(blocked, taskName)
				continue
			}

			states[clusterIndex].used = states[clusterIndex].used.Add(task.Resources)
			cluster := states[clusterIndex].cluster
			view := taskViews[taskName]
			view.Status = TaskRunning
			view.Cluster = cluster.Name
			taskViews[taskName] = view
			steps = append(steps, buildSimulationStep(len(steps), "scheduled", fmt.Sprintf("%s submitted to %s.", taskName, cluster.Name), taskName, cluster.Name, workflow, taskViews, states, blocked))

			running++
			scheduled = true
			go runTask(runCtx, executor, clusterIndex, cluster, task, results)
		}
		ready = append([]string(nil), blocked...)

		if running == 0 {
			err := fmt.Errorf("ready tasks cannot fit current cluster resources: %v", ready)
			return executionFailure(workflow, clusters, steps, err)
		}
		if scheduled {
			ready = sortReadyByCost(ready, g.tasks)
		}

		result := <-results
		running--

		task := g.tasks[result.taskName]
		states[result.clusterIndex].used = states[result.clusterIndex].used.Sub(task.Resources)
		view := taskViews[result.taskName]
		if result.err != nil {
			cancel()
			view.Status = TaskFailed
			taskViews[result.taskName] = view
			steps = append(steps, buildSimulationStep(len(steps), "failed", fmt.Sprintf("%s failed on %s: %v", result.taskName, result.cluster.Name, result.err), result.taskName, result.cluster.Name, workflow, taskViews, states, ready))
			return executionFailure(workflow, clusters, steps, result.err)
		}

		view.Status = TaskSucceeded
		taskViews[result.taskName] = view
		completed++
		steps = append(steps, buildSimulationStep(len(steps), "succeeded", fmt.Sprintf("%s completed on %s.", result.taskName, result.cluster.Name), result.taskName, result.cluster.Name, workflow, taskViews, states, ready))

		newReady := g.markCompleted(result.taskName)
		newReady = sortReadyByCost(newReady, g.tasks)
		ready = append(ready, newReady...)
		ready = sortReadyByCost(ready, g.tasks)
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

type taskExecutionResult struct {
	taskName     string
	clusterIndex int
	cluster      Cluster
	err          error
}

func runTask(ctx context.Context, executor Executor, clusterIndex int, cluster Cluster, task TaskSpec, results chan<- taskExecutionResult) {
	results <- taskExecutionResult{
		taskName:     task.Name,
		clusterIndex: clusterIndex,
		cluster:      cluster,
		err:          executor.RunTask(ctx, cluster, task),
	}
}

func sortReadyByCost(ready []string, tasks map[string]TaskSpec) []string {
	sorted := append([]string(nil), ready...)
	sort.Slice(sorted, func(i, j int) bool {
		left := tasks[sorted[i]]
		right := tasks[sorted[j]]
		leftCost := resourceCost(left.Resources)
		rightCost := resourceCost(right.Resources)
		if leftCost == rightCost {
			return left.Name < right.Name
		}
		return leftCost > rightCost
	})
	return sorted
}

func executionFailure(workflow Workflow, clusters []Cluster, steps []SimulationStep, err error) (ExecutionResult, error) {
	return ExecutionResult{
		Workflow: workflow,
		Clusters: clusters,
		Steps:    steps,
		Error:    err.Error(),
	}, err
}
