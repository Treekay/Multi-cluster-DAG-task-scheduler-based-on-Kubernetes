package scheduler

import "strings"

func NormalizeWorkflowCosts(workflow Workflow) Workflow {
	workflow.Tasks = append([]TaskSpec(nil), workflow.Tasks...)
	for i := range workflow.Tasks {
		workflow.Tasks[i].Resources = EstimateTaskResources(workflow.Tasks[i])
	}
	return workflow
}

func EstimateTaskResources(task TaskSpec) Resources {
	resources := task.Resources
	estimate := estimatedResourcesFromTaskShape(task)
	if resources.CPU <= 0 {
		resources.CPU = estimate.CPU
	}
	if resources.MemoryMiB <= 0 {
		resources.MemoryMiB = estimate.MemoryMiB
	}
	return resources
}

func estimatedResourcesFromTaskShape(task TaskSpec) Resources {
	text := strings.ToLower(task.Name + " " + task.Image + " " + strings.Join(task.Command, " "))

	switch {
	case strings.Contains(text, "train"), strings.Contains(text, "model"):
		return Resources{CPU: 4, MemoryMiB: 1024}
	case strings.Contains(text, "feature"), strings.Contains(text, "video"), strings.Contains(text, "encode"):
		return Resources{CPU: 2, MemoryMiB: 512}
	case strings.Contains(text, "evaluate"), strings.Contains(text, "validate"), strings.Contains(text, "test"):
		return Resources{CPU: 1, MemoryMiB: 256}
	default:
		return Resources{CPU: 1, MemoryMiB: 128}
	}
}

func resourceCost(resources Resources) int {
	return resources.CPU*1024 + resources.MemoryMiB
}
