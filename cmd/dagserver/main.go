package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/reconstruct/multi-cluster-dag-scheduler/internal/scheduler"
)

type defaultPayload struct {
	Workflow scheduler.Workflow  `json:"workflow"`
	Clusters []scheduler.Cluster `json:"clusters"`
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "HTTP listen address")
	workflowPath := flag.String("workflow", "examples/workflow.json", "default workflow JSON")
	clustersPath := flag.String("clusters", "examples/clusters.json", "default cluster config JSON")
	webDir := flag.String("web", "web", "web asset directory")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/default", defaultHandler(*workflowPath, *clustersPath))
	mux.HandleFunc("/api/simulate", simulateHandler)
	mux.Handle("/", http.FileServer(http.Dir(*webDir)))

	log.Printf("DAG scheduler UI listening on http://%s", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}

func defaultHandler(workflowPath, clustersPath string) http.HandlerFunc {
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

		writeJSON(w, defaultPayload{Workflow: workflow, Clusters: clusters})
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
