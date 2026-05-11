# Multi-cluster-DAG-task-scheduler-based-on-Kubernetes

This project reconstructs an undergraduate thesis topic: a Kubernetes-based
multi-cluster scheduler for DAG workloads.

## Goal

Build a scheduler that accepts a DAG workflow, tracks task dependencies, chooses
an appropriate Kubernetes cluster for each ready task, submits workloads, and
updates the workflow state until completion.

## MVP Scope

The first version focuses on a runnable scheduling core:

- Parse a workflow definition.
- Validate DAG dependencies and detect cycles.
- Track ready, running, completed, and failed tasks.
- Select a target cluster with enough available resources.
- Execute tasks through a pluggable executor.
- Provide a local simulator before connecting to real Kubernetes clusters.

## Architecture

```text
workflow spec -> DAG planner -> scheduler -> cluster selector -> executor
                                      |             |
                                      v             v
                                workflow state   cluster state
```

Planned stages:

1. Local scheduling core and simulator.
2. Kubernetes executor that creates Jobs in one selected cluster.
3. Multi-cluster kubeconfig support.
4. CRD/controller mode for production-style operation.
5. Metrics, retry policy, deadline handling, and scheduling experiments.

## Quick Start

Run the local simulator:

```powershell
go run ./cmd/dagscheduler -workflow examples/workflow.json
```

Run with Kubernetes clusters from your kubeconfig:

```powershell
go run ./cmd/dagscheduler `
  -executor kubernetes `
  -workflow examples/workflow.json `
  -clusters examples/clusters.json
```

Run tests:

```powershell
go test ./...
```

Start the local visualization console:

```powershell
go run ./cmd/dagserver
```

Then open [http://127.0.0.1:8080](http://127.0.0.1:8080). The console shows
the DAG, cluster resource usage, and the simulated scheduling event stream.

Run the visualization console with Docker:

```powershell
docker compose up --build
```

Then open [http://127.0.0.1:8080](http://127.0.0.1:8080). The container runs
one Go server that serves both the API and the frontend assets.

Make sure Docker Desktop is running before using Docker Compose. This container
starts the local visualization and simulation UI. It also includes `kubectl` and
mounts your local kubeconfig so the `Run Kubernetes` button can submit Jobs to
the contexts listed in `examples/clusters.json`.

## Kubernetes Test Setup

For local multi-cluster testing, create two kind clusters:

```powershell
kind create cluster --name dag-a
kind create cluster --name dag-b
kubectl config get-contexts
```

The example cluster config expects these kubeconfig contexts:

- `kind-dag-a`
- `kind-dag-b`

Each workflow task is converted to a Kubernetes `batch/v1 Job`. The scheduler
selects a cluster using the configured logical capacity, applies the Job with
`kubectl`, waits for completion, and then deletes the Job.
