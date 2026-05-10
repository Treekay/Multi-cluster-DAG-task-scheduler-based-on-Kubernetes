package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type CommandRunner interface {
	Run(ctx context.Context, args []string, stdin string) (string, error)
}

type KubectlRunner struct{}

func (KubectlRunner) Run(ctx context.Context, args []string, stdin string) (string, error) {
	command := exec.CommandContext(ctx, "kubectl", args...)
	if stdin != "" {
		command.Stdin = strings.NewReader(stdin)
	}
	output, err := command.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("kubectl %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

type KubernetesExecutor struct {
	runner  CommandRunner
	timeout time.Duration
}

func NewKubernetesExecutor(timeout time.Duration) KubernetesExecutor {
	return KubernetesExecutor{
		runner:  KubectlRunner{},
		timeout: timeout,
	}
}

func NewKubernetesExecutorWithRunner(runner CommandRunner, timeout time.Duration) KubernetesExecutor {
	return KubernetesExecutor{runner: runner, timeout: timeout}
}

func (e KubernetesExecutor) RunTask(ctx context.Context, cluster Cluster, task TaskSpec) error {
	if cluster.Context == "" {
		return fmt.Errorf("cluster %q has no kube context", cluster.Name)
	}
	if cluster.Namespace == "" {
		cluster.Namespace = "default"
	}
	if task.Image == "" {
		return fmt.Errorf("task %q image is required for kubernetes execution", task.Name)
	}

	jobName := jobName(task.Name)
	manifest, err := buildJobManifest(jobName, task)
	if err != nil {
		return err
	}

	baseArgs := []string{"--context", cluster.Context, "-n", cluster.Namespace}
	if _, err := e.runner.Run(ctx, append(baseArgs, "apply", "-f", "-"), manifest); err != nil {
		return err
	}
	defer e.runner.Run(context.Background(), append(baseArgs, "delete", "job", jobName, "--ignore-not-found=true"), "")

	waitTimeout := e.timeout
	if waitTimeout <= 0 {
		waitTimeout = 10 * time.Minute
	}

	_, err = e.runner.Run(ctx, append(baseArgs,
		"wait",
		"--for=condition=complete",
		"job/"+jobName,
		"--timeout="+formatKubectlTimeout(waitTimeout),
	), "")
	if err != nil {
		logs, _ := e.runner.Run(context.Background(), append(baseArgs, "logs", "job/"+jobName, "--tail=100"), "")
		if strings.TrimSpace(logs) != "" {
			return fmt.Errorf("%w\njob logs:\n%s", err, strings.TrimSpace(logs))
		}
		return err
	}

	return nil
}

func buildJobManifest(name string, task TaskSpec) (string, error) {
	if name == "" {
		return "", fmt.Errorf("job name is required")
	}
	if task.Image == "" {
		return "", fmt.Errorf("task image is required")
	}

	container := map[string]any{
		"name":  "task",
		"image": task.Image,
		"resources": map[string]any{
			"requests": resourceRequirements(task.Resources),
			"limits":   resourceRequirements(task.Resources),
		},
	}
	if len(task.Command) > 0 {
		container["command"] = task.Command
	}

	manifest := map[string]any{
		"apiVersion": "batch/v1",
		"kind":       "Job",
		"metadata": map[string]any{
			"name": name,
			"labels": map[string]string{
				"app.kubernetes.io/name":       "dag-scheduler",
				"app.kubernetes.io/managed-by": "dag-scheduler",
				"dag-scheduler/task":           name,
			},
		},
		"spec": map[string]any{
			"backoffLimit":            0,
			"ttlSecondsAfterFinished": 600,
			"template": map[string]any{
				"metadata": map[string]any{
					"labels": map[string]string{
						"app.kubernetes.io/name": "dag-scheduler",
						"dag-scheduler/task":     name,
					},
				},
				"spec": map[string]any{
					"restartPolicy": "Never",
					"containers":    []map[string]any{container},
				},
			},
		},
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func resourceRequirements(resources Resources) map[string]string {
	requirements := make(map[string]string)
	if resources.CPU > 0 {
		requirements["cpu"] = fmt.Sprintf("%d", resources.CPU)
	}
	if resources.MemoryMiB > 0 {
		requirements["memory"] = fmt.Sprintf("%dMi", resources.MemoryMiB)
	}
	return requirements
}

var invalidJobNameCharacters = regexp.MustCompile(`[^a-z0-9-]+`)

func jobName(taskName string) string {
	name := strings.ToLower(taskName)
	name = invalidJobNameCharacters.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")
	if name == "" {
		name = "task"
	}
	name = "dag-" + name
	if len(name) > 63 {
		name = strings.TrimRight(name[:63], "-")
	}
	return name
}

func formatKubectlTimeout(timeout time.Duration) string {
	seconds := int(timeout.Round(time.Second).Seconds())
	if seconds < 1 {
		seconds = 1
	}
	return fmt.Sprintf("%ds", seconds)
}
