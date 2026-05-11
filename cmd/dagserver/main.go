package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/reconstruct/multi-cluster-dag-scheduler/internal/scheduler"
)

type defaultPayload struct {
	Workflow scheduler.Workflow  `json:"workflow"`
	Clusters []scheduler.Cluster `json:"clusters"`
	Examples []workflowExample   `json:"examples"`
}

type workflowExample struct {
	Name     string             `json:"name"`
	Path     string             `json:"path"`
	Workflow scheduler.Workflow `json:"workflow"`
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "HTTP listen address")
	workflowPath := flag.String("workflow", "examples/workflow.json", "default workflow JSON")
	workflowsDir := flag.String("workflows", "examples/workflows", "directory containing workflow examples")
	clustersPath := flag.String("clusters", "examples/clusters.json", "default cluster config JSON")
	kubernetesTaskTimeout := flag.Duration("kubernetes-task-timeout", 10*time.Minute, "timeout for each Kubernetes Job")
	webDir := flag.String("web", "web", "web asset directory")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/default", defaultHandler(*workflowPath, *workflowsDir, *clustersPath))
	mux.HandleFunc("/api/simulate", simulateHandler)
	mux.HandleFunc("/api/kubernetes/run", kubernetesRunHandler(*kubernetesTaskTimeout))
	mux.Handle("/", http.FileServer(http.Dir(*webDir)))

	log.Printf("DAG scheduler UI listening on http://%s", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}

func defaultHandler(workflowPath, workflowsDir, clustersPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		workflow, err := loadWorkflow(workflowPath)
		if err != nil {
			writeError(w, fmt.Errorf("load workflow: %w", err), http.StatusInternalServerError)
			return
		}
		clusters, err := scheduler.LoadClusterConfig(clustersPath)
		if err != nil {
			writeError(w, fmt.Errorf("load clusters: %w", err), http.StatusInternalServerError)
			return
		}
		examples, err := loadWorkflowExamples(workflowPath, workflowsDir)
		if err != nil {
			writeError(w, fmt.Errorf("load workflow examples: %w", err), http.StatusInternalServerError)
			return
		}

		writeJSON(w, defaultPayload{Workflow: workflow, Clusters: clusters, Examples: examples})
	}
}

func simulateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request scheduler.SimulationRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, fmt.Errorf("decode simulation request: %w", err), http.StatusBadRequest)
		return
	}

	simulation, err := scheduler.Simulate(request.Workflow, request.Clusters)
	if err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}

	writeJSON(w, simulation)
}

func kubernetesRunHandler(taskTimeout time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var request scheduler.ExecutionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeError(w, fmt.Errorf("decode Kubernetes execution request: %w", err), http.StatusBadRequest)
			return
		}

		workflowTimeout := taskTimeout * time.Duration(max(1, len(request.Workflow.Tasks)))
		ctx, cancel := context.WithTimeout(r.Context(), workflowTimeout)
		defer cancel()

		result, err := scheduler.ExecuteWorkflow(ctx, request.Workflow, request.Clusters, scheduler.NewKubernetesExecutor(taskTimeout))
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			writeJSON(w, result)
			return
		}

		writeJSON(w, result)
	}
}

func loadWorkflow(path string) (scheduler.Workflow, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return scheduler.Workflow{}, err
	}

	var workflow scheduler.Workflow
	if err := json.Unmarshal(data, &workflow); err != nil {
		return scheduler.Workflow{}, err
	}

	return workflow, nil
}

func loadWorkflowExamples(defaultPath, workflowsDir string) ([]workflowExample, error) {
	examples := []workflowExample{}
	seen := map[string]struct{}{}

	addExample := func(path string) error {
		cleanPath := filepath.Clean(path)
		if _, ok := seen[cleanPath]; ok {
			return nil
		}
		workflow, err := loadWorkflow(cleanPath)
		if err != nil {
			return err
		}
		examples = append(examples, workflowExample{
			Name:     workflow.Name,
			Path:     cleanPath,
			Workflow: workflow,
		})
		seen[cleanPath] = struct{}{}
		return nil
	}

	if err := addExample(defaultPath); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(filepath.Clean(workflowsDir))
	if err != nil {
		if os.IsNotExist(err) {
			return examples, nil
		}
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		if err := addExample(filepath.Join(workflowsDir, entry.Name())); err != nil {
			return nil, err
		}
	}

	return examples, nil
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write response: %v", err)
	}
}

func writeError(w http.ResponseWriter, err error, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
