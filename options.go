package ptm

import (
	"context"
	"time"
)

type Option func(*Task)

func WithName(name string) Option {
	return func(t *Task) {
		t.Name = name
	}
}

func WithContext(ctx context.Context) Option {
	return func(t *Task) {
		t.ctx, t.cancel = context.WithCancel(ctx)
	}
}

func WithMaxRuntime(maxRuntime time.Duration) Option {
	return func(t *Task) {
		t.maxRuntime = maxRuntime
	}
}

func WithPriority(priority int) Option {
	return func(t *Task) {
		t.Priority = priority
	}
}

func WithValue(value any) Option {
	return func(t *Task) {
		t.Value = value
	}
}

func WithMaxRetry(maxRetry int, delay time.Duration) Option {
	return func(t *Task) {
		t.maxRetry = maxRetry
		t.retryLeft = maxRetry
		t.retryDelay = delay
	}
}

func WithTimeout(timeout time.Duration) Option {
	return func(t *Task) {
		t.watchdogTimeout = timeout
	}
}

func WithRunFunc(runFn TaskRunFunc) Option {
	return func(t *Task) {
		t.run = runFn
	}
}
