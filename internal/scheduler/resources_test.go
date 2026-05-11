package scheduler

import (
	"context"
	"strings"
	"testing"
)

type resourceRunner struct{}

func (resourceRunner) Run(ctx context.Context, args []string, stdin string) (string, error) {
	command := strings.Join(args, " ")
	if strings.Contains(command, "get nodes") {
		return `{
			"items": [
				{"status": {"allocatable": {"cpu": "2", "memory": "2Gi"}}},
				{"status": {"allocatable": {"cpu": "1900m", "memory": "1536Mi"}}}
			]
		}`, nil
	}
	if strings.Contains(command, "get pods") {
		return `{
			"items": [
				{"spec": {"containers": [{"resources": {"requests": {"cpu": "500m", "memory": "256Mi"}}}]}, "status": {"phase": "Running"}},
				{"spec": {"containers": [{"resources": {"requests": {"cpu": "1", "memory": "512Mi"}}}]}, "status": {"phase": "Succeeded"}}
			]
		}`, nil
	}
	return `{}`, nil
}

func TestKubernetesResourceInspectorUsesAllocatableMinusRunningRequests(t *testing.T) {
	inspector := NewKubernetesResourceInspectorWithRunner(resourceRunner{})
	clusters, err := inspector.InspectClusterResources(context.Background(), []Cluster{
		{Name: "cluster-a", Context: "kind-a", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("inspect resources: %v", err)
	}

	got := clusters[0].Capacity
	want := Resources{CPU: 3, MemoryMiB: 3328}
	if got != want {
		t.Fatalf("capacity = %+v, want %+v", got, want)
	}
}

func TestEstimateTaskResourcesKeepsExplicitValues(t *testing.T) {
	task := TaskSpec{Name: "train-model", Resources: Resources{CPU: 6, MemoryMiB: 2048}}
	if got := EstimateTaskResources(task); got != task.Resources {
		t.Fatalf("resources = %+v, want %+v", got, task.Resources)
	}
}
