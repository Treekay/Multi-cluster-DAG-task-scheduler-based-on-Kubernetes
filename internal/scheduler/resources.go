package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type ClusterResourceInspector interface {
	InspectClusterResources(ctx context.Context, clusters []Cluster) ([]Cluster, error)
}

type KubernetesResourceInspector struct {
	runner CommandRunner
}

func NewKubernetesResourceInspector() KubernetesResourceInspector {
	return KubernetesResourceInspector{runner: KubectlRunner{}}
}

func NewKubernetesResourceInspectorWithRunner(runner CommandRunner) KubernetesResourceInspector {
	return KubernetesResourceInspector{runner: runner}
}

func (i KubernetesResourceInspector) InspectClusterResources(ctx context.Context, clusters []Cluster) ([]Cluster, error) {
	observed := append([]Cluster(nil), clusters...)
	for index, cluster := range observed {
		if cluster.Context == "" {
			return nil, fmt.Errorf("cluster %q has no kube context", cluster.Name)
		}
		if cluster.Namespace == "" {
			cluster.Namespace = "default"
		}

		capacity, err := i.nodeAllocatable(ctx, cluster)
		if err != nil {
			return nil, fmt.Errorf("inspect %s nodes: %w", cluster.Name, err)
		}
		used, err := i.podRequests(ctx, cluster)
		if err != nil {
			return nil, fmt.Errorf("inspect %s pods: %w", cluster.Name, err)
		}

		available := capacity.Sub(used)
		if available.CPU < 0 {
			available.CPU = 0
		}
		if available.MemoryMiB < 0 {
			available.MemoryMiB = 0
		}
		observed[index].Capacity = available
	}
	return observed, nil
}

func (i KubernetesResourceInspector) nodeAllocatable(ctx context.Context, cluster Cluster) (Resources, error) {
	output, err := i.runner.Run(ctx, []string{"--context", cluster.Context, "get", "nodes", "-o", "json"}, "")
	if err != nil {
		return Resources{}, err
	}

	var nodes kubernetesNodeList
	if err := json.Unmarshal([]byte(output), &nodes); err != nil {
		return Resources{}, err
	}

	var total Resources
	for _, node := range nodes.Items {
		total = total.Add(Resources{
			CPU:       parseCPU(node.Status.Allocatable["cpu"]),
			MemoryMiB: parseMemoryMiB(node.Status.Allocatable["memory"]),
		})
	}
	if total.CPU <= 0 || total.MemoryMiB <= 0 {
		return Resources{}, fmt.Errorf("no allocatable node resources found")
	}
	return total, nil
}

func (i KubernetesResourceInspector) podRequests(ctx context.Context, cluster Cluster) (Resources, error) {
	output, err := i.runner.Run(ctx, []string{"--context", cluster.Context, "get", "pods", "-A", "-o", "json"}, "")
	if err != nil {
		return Resources{}, err
	}

	var pods kubernetesPodList
	if err := json.Unmarshal([]byte(output), &pods); err != nil {
		return Resources{}, err
	}

	var total Resources
	for _, pod := range pods.Items {
		if pod.Status.Phase == "Succeeded" || pod.Status.Phase == "Failed" {
			continue
		}
		for _, container := range pod.Spec.Containers {
			total = total.Add(Resources{
				CPU:       parseCPU(container.Resources.Requests["cpu"]),
				MemoryMiB: parseMemoryMiB(container.Resources.Requests["memory"]),
			})
		}
	}
	return total, nil
}

func parseCPU(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if strings.HasSuffix(value, "m") {
		milli, err := strconv.Atoi(strings.TrimSuffix(value, "m"))
		if err != nil {
			return 0
		}
		return ceilDiv(milli, 1000)
	}
	cores, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return int(cores + 0.999999)
}

func parseMemoryMiB(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}

	units := []struct {
		suffix string
		scale  float64
	}{
		{"Ki", 1.0 / 1024.0},
		{"Mi", 1},
		{"Gi", 1024},
		{"Ti", 1024 * 1024},
		{"K", 1.0 / 1000.0},
		{"M", 1000.0 / 1024.0},
		{"G", 1000.0 * 1000.0 / 1024.0},
	}
	for _, unit := range units {
		if strings.HasSuffix(value, unit.suffix) {
			number, err := strconv.ParseFloat(strings.TrimSuffix(value, unit.suffix), 64)
			if err != nil {
				return 0
			}
			return int(number*unit.scale + 0.999999)
		}
	}

	bytes, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return int(bytes/(1024*1024) + 0.999999)
}

func ceilDiv(value, divisor int) int {
	if value <= 0 {
		return 0
	}
	return (value + divisor - 1) / divisor
}

type kubernetesNodeList struct {
	Items []struct {
		Status struct {
			Allocatable map[string]string `json:"allocatable"`
		} `json:"status"`
	} `json:"items"`
}

type kubernetesPodList struct {
	Items []struct {
		Spec struct {
			Containers []struct {
				Resources struct {
					Requests map[string]string `json:"requests"`
				} `json:"resources"`
			} `json:"containers"`
		} `json:"spec"`
		Status struct {
			Phase string `json:"phase"`
		} `json:"status"`
	} `json:"items"`
}
