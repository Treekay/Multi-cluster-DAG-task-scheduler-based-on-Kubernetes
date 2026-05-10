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

Run tests:

```powershell
go test ./...
```
