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
	clustersPath := flag.String("clusters", "", "path to cluster config JSON")
	executorMode := flag.String("executor", "simulate", "executor mode: simulate or kubernetes")
	taskTimeout := flag.Duration("task-timeout", 10*time.Minute, "timeout for each Kubernetes Job")
	flag.Parse()

	workflow, err := loadWorkflow(*workflowPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load workflow: %v\n", err)
		os.Exit(1)
	}

	clusters, err := loadClusters(*clustersPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load clusters: %v\n", err)
		os.Exit(1)
	}

	var executor scheduler.Executor
	switch *executorMode {
	case "simulate":
		executor = scheduler.NewSimulatedExecutor(350 * time.Millisecond)
	case "kubernetes":
		if *clustersPath == "" {
			fmt.Fprintln(os.Stderr, "kubernetes executor requires -clusters")
			os.Exit(1)
		}
		executor = scheduler.NewKubernetesExecutor(*taskTimeout)
	default:
		fmt.Fprintf(os.Stderr, "unknown executor %q\n", *executorMode)
		os.Exit(1)
	}

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

func loadClusters(path string) ([]scheduler.Cluster, error) {
	if path != "" {
		return scheduler.LoadClusterConfig(path)
	}

	return []scheduler.Cluster{
		{Name: "cluster-a", Capacity: scheduler.Resources{CPU: 4, MemoryMiB: 4096}},
		{Name: "cluster-b", Capacity: scheduler.Resources{CPU: 8, MemoryMiB: 8192}},
	}, nil
}
