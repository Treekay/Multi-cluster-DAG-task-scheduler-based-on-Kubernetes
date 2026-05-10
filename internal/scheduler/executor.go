package scheduler

import (
	"context"
	"time"
)

type Executor interface {
	RunTask(ctx context.Context, cluster Cluster, task TaskSpec) error
}

type SimulatedExecutor struct {
	delay time.Duration
}

func NewSimulatedExecutor(delay time.Duration) SimulatedExecutor {
	return SimulatedExecutor{delay: delay}
}

func (e SimulatedExecutor) RunTask(ctx context.Context, cluster Cluster, task TaskSpec) error {
	timer := time.NewTimer(e.delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
