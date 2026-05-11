package scheduler

import "testing"

func TestSimulateBuildsTimeline(t *testing.T) {
	workflow := Workflow{
		Name: "etl",
		Tasks: []TaskSpec{
			{Name: "extract", Resources: Resources{CPU: 1, MemoryMiB: 128}},
			{Name: "transform", DependsOn: []string{"extract"}, Resources: Resources{CPU: 2, MemoryMiB: 256}},
			{Name: "load", DependsOn: []string{"transform"}, Resources: Resources{CPU: 1, MemoryMiB: 128}},
		},
	}
	clusters := []Cluster{
		{Name: "small", Capacity: Resources{CPU: 1, MemoryMiB: 512}},
		{Name: "large", Capacity: Resources{CPU: 4, MemoryMiB: 1024}},
	}

	simulation, err := Simulate(workflow, clusters)
	if err != nil {
		t.Fatalf("simulate: %v", err)
	}

	if len(simulation.Steps) == 0 {
		t.Fatal("expected simulation steps")
	}
	last := simulation.Steps[len(simulation.Steps)-1]
	if last.Type != "finished" {
		t.Fatalf("last step type = %q, want finished", last.Type)
	}
	for _, task := range last.Tasks {
		if task.Status != TaskSucceeded {
			t.Fatalf("task %s status = %s, want %s", task.Name, task.Status, TaskSucceeded)
		}
	}
}

func TestSimulateShowsIndependentReadyTasksRunningTogether(t *testing.T) {
	workflow := Workflow{
		Name: "parallel",
		Tasks: []TaskSpec{
			{Name: "a", Resources: Resources{CPU: 1, MemoryMiB: 128}},
			{Name: "b", Resources: Resources{CPU: 1, MemoryMiB: 128}},
		},
	}
	clusters := []Cluster{{Name: "cluster-a", Capacity: Resources{CPU: 2, MemoryMiB: 512}}}

	simulation, err := Simulate(workflow, clusters)
	if err != nil {
		t.Fatalf("simulate: %v", err)
	}

	foundConcurrentRunning := false
	for _, step := range simulation.Steps {
		running := 0
		for _, task := range step.Tasks {
			if task.Status == TaskRunning {
				running++
			}
		}
		if running == 2 {
			foundConcurrentRunning = true
			break
		}
	}
	if !foundConcurrentRunning {
		t.Fatal("expected a simulation step with both independent tasks running")
	}
}
