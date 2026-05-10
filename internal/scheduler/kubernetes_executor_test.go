package scheduler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

type recordedCommand struct {
	args  []string
	stdin string
}

type recordingRunner struct {
	commands []recordedCommand
}

func (r *recordingRunner) Run(ctx context.Context, args []string, stdin string) (string, error) {
	r.commands = append(r.commands, recordedCommand{
		args:  append([]string(nil), args...),
		stdin: stdin,
	})
	return "", nil
}

func TestKubernetesExecutorCreatesWaitsAndDeletesJob(t *testing.T) {
	runner := &recordingRunner{}
	executor := NewKubernetesExecutorWithRunner(runner, 2*time.Second)

	cluster := Cluster{Name: "cluster-a", Context: "kind-a", Namespace: "dag", Capacity: Resources{CPU: 2, MemoryMiB: 1024}}
	task := TaskSpec{Name: "Extract Data", Image: "busybox:1.36", Command: []string{"sh", "-c", "echo ok"}, Resources: Resources{CPU: 1, MemoryMiB: 128}}
	if err := executor.RunTask(context.Background(), cluster, task); err != nil {
		t.Fatalf("run task: %v", err)
	}

	if len(runner.commands) != 3 {
		t.Fatalf("commands = %d, want 3", len(runner.commands))
	}
	if strings.Join(runner.commands[0].args, " ") != "--context kind-a -n dag apply -f -" {
		t.Fatalf("apply args = %q", strings.Join(runner.commands[0].args, " "))
	}
	if !strings.Contains(strings.Join(runner.commands[1].args, " "), "wait --for=condition=complete job/dag-extract-data --timeout=2s") {
		t.Fatalf("wait args = %q", strings.Join(runner.commands[1].args, " "))
	}
	if !strings.Contains(strings.Join(runner.commands[2].args, " "), "delete job dag-extract-data") {
		t.Fatalf("delete args = %q", strings.Join(runner.commands[2].args, " "))
	}
}

func TestBuildJobManifestIncludesContainerAndResources(t *testing.T) {
	manifest, err := buildJobManifest("dag-transform", TaskSpec{
		Name:      "transform",
		Image:     "busybox:1.36",
		Command:   []string{"sh", "-c", "echo transform"},
		Resources: Resources{CPU: 2, MemoryMiB: 256},
	})
	if err != nil {
		t.Fatalf("build manifest: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(manifest), &decoded); err != nil {
		t.Fatalf("manifest is not json: %v", err)
	}

	if decoded["kind"] != "Job" {
		t.Fatalf("kind = %v, want Job", decoded["kind"])
	}
	if !strings.Contains(manifest, `"image": "busybox:1.36"`) {
		t.Fatalf("manifest does not contain image: %s", manifest)
	}
	if !strings.Contains(manifest, `"memory": "256Mi"`) {
		t.Fatalf("manifest does not contain memory request: %s", manifest)
	}
}
