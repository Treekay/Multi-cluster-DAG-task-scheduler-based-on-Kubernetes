package scheduler

import (
	"encoding/json"
	"fmt"
	"os"
)

type ClusterConfig struct {
	Clusters []Cluster `json:"clusters"`
}

func LoadClusterConfig(path string) ([]Cluster, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config ClusterConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	if len(config.Clusters) == 0 {
		return nil, fmt.Errorf("cluster config must contain at least one cluster")
	}

	for i := range config.Clusters {
		cluster := &config.Clusters[i]
		if cluster.Name == "" {
			return nil, fmt.Errorf("cluster name is required")
		}
		if cluster.Namespace == "" {
			cluster.Namespace = "default"
		}
		if cluster.Capacity.CPU <= 0 && cluster.Context == "" {
			return nil, fmt.Errorf("cluster %q cpu capacity must be positive when no kube context is configured", cluster.Name)
		}
		if cluster.Capacity.MemoryMiB <= 0 && cluster.Context == "" {
			return nil, fmt.Errorf("cluster %q memory capacity must be positive when no kube context is configured", cluster.Name)
		}
	}

	return config.Clusters, nil
}
