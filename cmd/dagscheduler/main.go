package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/reconstruct/multi-cluster-dag-scheduler/internal/scheduler"
)

func main() {
	workflowPath := flag.String("workflow", "examples/workflow.json", "path to workflow JSON")
	flag.Parse()

	workflow, err := loadWorkflow(*workflowPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load workflow: %v\n", err)
		os.Exit(1)
	}

	clusters := []scheduler.Cluster{
		{Name: "cluster-a", Capacity: scheduler.Resources{CPU: 4, MemoryMiB: 4096}},
		{Name: "cluster-b", Capacity: scheduler.Resources{CPU: 8, MemoryMiB: 8192}},
	}

	executor := scheduler.NewSimulatedExecutor(350 * time.Millisecond)
	engine := scheduler.NewEngine(clusters, executor)

	result, err := engine.Run(context.Background(), workflow)
	if err != nil {
		fmt.Fprintf(os.Stderr, "schedule workflow: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("workflow %q finished\n", result.WorkflowName)
	for _, task := range result.Tasks {
		fmt.Printf("- %s -> %s on %s\n", task.Name, task.Status, task.Cluster)
	}
}

func loadWorkflow(path string) (scheduler.Workflow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return scheduler.Workflow{}, err
	}

	var workflow scheduler.Workflow
	if err := json.Unmarshal(data, &workflow); err != nil {
		return scheduler.Workflow{}, err
	}

	return workflow, nil
}
