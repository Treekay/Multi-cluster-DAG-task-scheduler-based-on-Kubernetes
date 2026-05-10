package scheduler

import (
	"context"
	"testing"
	"time"
)

func TestEngineRunsWorkflowInDependencyOrder(t *testing.T) {
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

	engine := NewEngine(clusters, NewSimulatedExecutor(time.Nanosecond))
	result, err := engine.Run(context.Background(), workflow)
	if err != nil {
		t.Fatalf("run workflow: %v", err)
	}

	for _, task := range result.Tasks {
		if task.Status != TaskSucceeded {
			t.Fatalf("task %s status = %s, want %s", task.Name, task.Status, TaskSucceeded)
		}
	}

	if result.Tasks[1].Cluster != "large" {
		t.Fatalf("transform cluster = %s, want large", result.Tasks[1].Cluster)
	}
}

func TestEngineRejectsUnschedulableTask(t *testing.T) {
	workflow := Workflow{
		Name: "too-big",
		Tasks: []TaskSpec{
			{Name: "huge", Resources: Resources{CPU: 99, MemoryMiB: 128}},
		},
	}
	clusters := []Cluster{{Name: "small", Capacity: Resources{CPU: 1, MemoryMiB: 512}}}

	engine := NewEngine(clusters, NewSimulatedExecutor(time.Nanosecond))
	if _, err := engine.Run(context.Background(), workflow); err == nil {
		t.Fatal("expected unschedulable task error")
	}
}
