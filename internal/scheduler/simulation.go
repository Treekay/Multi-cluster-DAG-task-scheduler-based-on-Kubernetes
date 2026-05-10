package scheduler

import (
	"fmt"
	"sort"
)

type SimulationRequest struct {
	Workflow Workflow  `json:"workflow"`
	Clusters []Cluster `json:"clusters"`
}

type Simulation struct {
	Workflow Workflow         `json:"workflow"`
	Clusters []Cluster        `json:"clusters"`
	Steps    []SimulationStep `json:"steps"`
}

type SimulationStep struct {
	Index    int           `json:"index"`
	Type     string        `json:"type"`
	Message  string        `json:"message"`
	Task     string        `json:"task,omitempty"`
	Cluster  string        `json:"cluster,omitempty"`
	Tasks    []TaskView    `json:"tasks"`
	Clusters []ClusterView `json:"clusters"`
	Ready    []string      `json:"ready"`
}

type TaskView struct {
	Name      string     `json:"name"`
	Status    TaskStatus `json:"status"`
	Cluster   string     `json:"cluster,omitempty"`
	DependsOn []string   `json:"dependsOn"`
	CPU       int        `json:"cpu"`
	MemoryMiB int        `json:"memoryMiB"`
}

type ClusterView struct {
	Name           string `json:"name"`
	Context        string `json:"context,omitempty"`
	Namespace      string `json:"namespace,omitempty"`
	CPUCapacity    int    `json:"cpuCapacity"`
	CPUUsed        int    `json:"cpuUsed"`
	MemoryCapacity int    `json:"memoryCapacity"`
	MemoryUsed     int    `json:"memoryUsed"`
}

func Simulate(workflow Workflow, clusters []Cluster) (Simulation, error) {
	g, err := buildGraph(workflow)
	if err != nil {
		return Simulation{}, err
	}
	if len(clusters) == 0 {
		return Simulation{}, fmt.Errorf("at least one cluster is required")
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
	steps := []SimulationStep{buildSimulationStep(0, "init", "Workflow accepted and DAG dependencies validated.", "", "", workflow, taskViews, states, ready)}

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
			return Simulation{}, fmt.Errorf("task %q: %w", taskName, err)
		}

		states[clusterIndex].used = states[clusterIndex].used.Add(task.Resources)
		cluster := states[clusterIndex].cluster
		view := taskViews[taskName]
		view.Status = TaskRunning
		view.Cluster = cluster.Name
		taskViews[taskName] = view
		steps = append(steps, buildSimulationStep(len(steps), "scheduled", fmt.Sprintf("%s scheduled to %s.", taskName, cluster.Name), taskName, cluster.Name, workflow, taskViews, states, ready))

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
		return Simulation{}, fmt.Errorf("workflow stopped before all tasks completed")
	}

	steps = append(steps, buildSimulationStep(len(steps), "finished", "Workflow simulation finished.", "", "", workflow, taskViews, states, nil))
	return Simulation{Workflow: workflow, Clusters: clusters, Steps: steps}, nil
}

func buildSimulationStep(index int, eventType, message, task, cluster string, workflow Workflow, taskViews map[string]TaskView, states []clusterState, ready []string) SimulationStep {
	return SimulationStep{
		Index:    index,
		Type:     eventType,
		Message:  message,
		Task:     task,
		Cluster:  cluster,
		Tasks:    orderedTaskViews(workflow, taskViews),
		Clusters: clusterViews(states),
		Ready:    append([]string(nil), ready...),
	}
}

func orderedTaskViews(workflow Workflow, taskViews map[string]TaskView) []TaskView {
	tasks := make([]TaskView, 0, len(workflow.Tasks))
	for _, task := range workflow.Tasks {
		tasks = append(tasks, taskViews[task.Name])
	}
	return tasks
}

func clusterViews(states []clusterState) []ClusterView {
	views := make([]ClusterView, 0, len(states))
	for _, state := range states {
		views = append(views, ClusterView{
			Name:           state.cluster.Name,
			Context:        state.cluster.Context,
			Namespace:      state.cluster.Namespace,
			CPUCapacity:    state.cluster.Capacity.CPU,
			CPUUsed:        state.used.CPU,
			MemoryCapacity: state.cluster.Capacity.MemoryMiB,
			MemoryUsed:     state.used.MemoryMiB,
		})
	}
	return views
}

func selectCluster(states []clusterState, required Resources) (int, error) {
	bestIndex := -1
	bestFreeCPU := -1

	for i, state := range states {
		free := state.cluster.Capacity.Sub(state.used)
		if !free.Fits(required) {
			continue
		}
		if free.CPU > bestFreeCPU {
			bestIndex = i
			bestFreeCPU = free.CPU
		}
	}

	if bestIndex == -1 {
		return 0, fmt.Errorf("no cluster has enough free resources")
	}
	return bestIndex, nil
}
