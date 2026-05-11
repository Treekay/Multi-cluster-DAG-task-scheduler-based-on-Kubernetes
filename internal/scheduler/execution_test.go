package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"
)

type recordingExecutor struct {
	mu   sync.Mutex
	runs []string
}

func (e *recordingExecutor) RunTask(ctx context.Context, cluster Cluster, task TaskSpec) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.runs = append(e.runs, cluster.Name+"/"+task.Name)
	return nil
}

func TestExecuteWorkflowRunsExecutorAndBuildsSteps(t *testing.T) {
	workflow := Workflow{
		Name: "etl",
		Tasks: []TaskSpec{
			{Name: "extract", Resources: Resources{CPU: 1, MemoryMiB: 128}},
			{Name: "load", DependsOn: []string{"extract"}, Resources: Resources{CPU: 1, MemoryMiB: 128}},
		},
	}
	clusters := []Cluster{{Name: "cluster-a", Capacity: Resources{CPU: 2, MemoryMiB: 512}}}
	executor := &recordingExecutor{}

	result, err := ExecuteWorkflow(context.Background(), workflow, clusters, executor)
	if err != nil {
		t.Fatalf("execute workflow: %v", err)
	}

	if len(executor.runs) != 2 {
		t.Fatalf("executor runs = %d, want 2", len(executor.runs))
	}
	if result.Steps[len(result.Steps)-1].Type != "finished" {
		t.Fatalf("last step = %q, want finished", result.Steps[len(result.Steps)-1].Type)
	}
}

type slowExecutor struct {
	delay time.Duration
}

func (e slowExecutor) RunTask(ctx context.Context, cluster Cluster, task TaskSpec) error {
	select {
	case <-time.After(e.delay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestExecuteWorkflowRunsIndependentReadyTasksConcurrently(t *testing.T) {
	workflow := Workflow{
		Name: "parallel",
		Tasks: []TaskSpec{
			{Name: "a", Resources: Resources{CPU: 1, MemoryMiB: 128}},
			{Name: "b", Resources: Resources{CPU: 1, MemoryMiB: 128}},
		},
	}
	clusters := []Cluster{{Name: "cluster-a", Capacity: Resources{CPU: 2, MemoryMiB: 512}}}

	started := time.Now()
	_, err := ExecuteWorkflow(context.Background(), workflow, clusters, slowExecutor{delay: 120 * time.Millisecond})
	if err != nil {
		t.Fatalf("execute workflow: %v", err)
	}

	if elapsed := time.Since(started); elapsed > 220*time.Millisecond {
		t.Fatalf("execution took %s, expected independent tasks to overlap", elapsed)
	}
}
