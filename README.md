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
- Estimate missing task resource cost from task shape, then select a target
  cluster with enough available resources.
- Execute independent ready tasks concurrently when cluster resources allow it.
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
Use the workflow selector to switch between the built-in demo DAGs. `Run
Simulation` stays local and does not touch Kubernetes. `Run Kubernetes` submits
each DAG task as a real Kubernetes `Job` that runs a lightweight `busybox`
command.

Run the visualization console with Docker against real clusters:

```powershell
New-Item -ItemType Directory -Force config
Copy-Item examples/clusters.example.json config/clusters.json
# Edit config/clusters.json so each context matches your kubeconfig.
docker compose up --build
```

Then open [http://127.0.0.1:8080](http://127.0.0.1:8080). The container runs
one Go server that serves both the API and the frontend assets.

Make sure Docker Desktop is running before using Docker Compose. This container
starts the local visualization and simulation UI. It includes `kubectl`, mounts
your kubeconfig, and reads cluster metadata from `config/clusters.json`.

Use a non-default kubeconfig path if needed:

```powershell
$env:KUBECONFIG_DIR = "C:\path\to\directory-containing-config"
docker compose up --build
```

Demo workflows live in:

- `examples/workflow.json`
- `examples/workflows/data-lakehouse.json`
- `examples/workflows/ml-training.json`
- `examples/workflows/video-analytics.json`

## Kubernetes Test Setup

For local multi-cluster testing, create two kind clusters:

```powershell
kind create cluster --name dag-a
kind create cluster --name dag-b
kubectl config get-contexts
```

Start the Docker demo with the kind-specific compose override:

```powershell
docker compose -f docker-compose.yml -f docker-compose.kind.yml up --build
```

The example cluster config expects these kubeconfig contexts:

- `kind-dag-a`
- `kind-dag-b`

Each workflow task is converted to a Kubernetes `batch/v1 Job`. Before running,
the scheduler asks Kubernetes for node allocatable resources and current Pod
requests, uses that as the available cluster capacity, schedules all ready tasks
that fit, waits for completion, and then deletes finished Jobs.

## Scheduling Strategy

The current scheduler uses a resource-aware DAG strategy:

- DAG validation detects missing dependencies and cycles.
- Tasks with no unfinished dependencies enter the ready queue.
- Missing task CPU or memory is estimated from task name, image, and command.
  Explicit `resources` in workflow JSON always win.
- Ready tasks are sorted by estimated resource cost, largest first.
- A cluster is selected with a best-fit rule: the task must fit the currently
  available CPU and memory, then the scheduler chooses the cluster with the
  smallest remaining resource slack.
- All ready tasks that fit are submitted concurrently. Resources are reserved
  when a task is submitted and released when its Kubernetes Job finishes.

This is still a heuristic scheduler, not an optimizer. It is designed to show
real multi-cluster DAG behavior clearly while avoiding obvious waste. Future
research extensions can add HEFT, Min-Min/Max-Min, deadlines, data locality,
historical runtime prediction, retries, and queue priorities.

## Real Cluster Setup

To use the scheduler with someone else's Kubernetes clusters:

1. Make sure their kubeconfig contains one context per target cluster.
2. Copy `examples/clusters.example.json` to `config/clusters.json`.
3. Set each `context` value to an actual kubeconfig context.
4. Set `namespace` to the namespace where workflow Jobs should run.
5. Create that namespace in every target cluster.
6. Grant the kubeconfig identity permission to manage Jobs and read Pods.

For Kubernetes execution, `capacity` in `config/clusters.json` is only a
fallback and demo value. The scheduler refreshes actual available resources
from Kubernetes before submitting Jobs.

Minimum namespace-scoped RBAC:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: dag-jobs
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: dag-scheduler-job-runner
  namespace: dag-jobs
rules:
  - apiGroups: ["batch"]
    resources: ["jobs"]
    verbs: ["create", "get", "list", "watch", "delete"]
  - apiGroups: [""]
    resources: ["pods", "pods/log"]
    verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: dag-scheduler-job-runner
  namespace: dag-jobs
subjects:
  - kind: User
    name: replace-with-your-kubeconfig-user
    apiGroup: rbac.authorization.k8s.io
roleRef:
  kind: Role
  name: dag-scheduler-job-runner
  apiGroup: rbac.authorization.k8s.io
```

For actual free-resource estimation, the scheduler also needs to read Nodes and
Pods across the cluster:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: dag-scheduler-resource-reader
rules:
  - apiGroups: [""]
    resources: ["nodes", "pods"]
    verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: dag-scheduler-resource-reader
subjects:
  - kind: User
    name: replace-with-your-kubeconfig-user
    apiGroup: rbac.authorization.k8s.io
roleRef:
  kind: ClusterRole
  name: dag-scheduler-resource-reader
  apiGroup: rbac.authorization.k8s.io
```

Before opening the UI, check access from the same kubeconfig:

```powershell
kubectl --context your-prod-east-context -n dag-jobs auth can-i create jobs
kubectl --context your-prod-east-context -n dag-jobs auth can-i get pods
kubectl --context your-prod-east-context auth can-i list nodes
kubectl --context your-prod-east-context auth can-i list pods --all-namespaces
```

Then run:

```powershell
docker compose up --build
```

`Run Simulation` never touches Kubernetes. `Run Kubernetes` submits real Jobs to
the configured contexts and namespaces.

## Notes For Production

This project currently runs as a local control-plane UI that shells out to
`kubectl`. For a production deployment, the next step is to add a Helm chart and
an in-cluster mode that uses a Kubernetes ServiceAccount instead of mounting a
developer kubeconfig.
