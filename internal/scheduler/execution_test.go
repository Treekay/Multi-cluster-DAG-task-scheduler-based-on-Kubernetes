package scheduler

import (
	"context"
	"testing"
)

type recordingExecutor struct {
	runs []string
}

func (e *recordingExecutor) RunTask(ctx context.Context, cluster Cluster, task TaskSpec) error {
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
