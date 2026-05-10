package scheduler

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadClusterConfigDefaultsNamespace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clusters.json")
	content := `{
		"clusters": [
			{
				"name": "cluster-a",
				"context": "kind-a",
				"capacity": {"cpu": 2, "memoryMiB": 1024}
			}
		]
	}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	clusters, err := LoadClusterConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if clusters[0].Namespace != "default" {
		t.Fatalf("namespace = %q, want default", clusters[0].Namespace)
	}
}
