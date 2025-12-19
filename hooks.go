package ptm

import (
	"log/slog"
	"time"
)

type (
	OnStartFunc = func(name string)
	OnRetryFunc = func(name string, retryCount int, delay time.Duration, err error)
	OnStopFunc  = func(name string, reason error)
)

type Callback func(name string, args ...any)

type Callbacks struct {
	OnStart   OnStartFunc
	OnRetry   OnRetryFunc
	OnStop    OnStopFunc
	callbacks map[string]Callback
}

func NewDefaultCallbacks() Callbacks {
	return Callbacks{
		OnStart: func(name string) {
			slog.Debug("task starting...", "name", name)
		},
		OnRetry: func(name string, retryCount int, delay time.Duration, err error) {
			slog.Debug("task failed, retrying", "name", name, "retryCount", retryCount, "delay", delay, "err", err)
		},
		OnStop: func(name string, reason error) {
			slog.Debug("task stopped", "name", name, "reason", reason)
		},
		callbacks: make(map[string]Callback),
	}
}
