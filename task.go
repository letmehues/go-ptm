package ptm

import (
	"context"
	"sync"
	"time"
)

type TaskRunFunc func(ctx context.Context, task *Task) error

type TaskInfo struct {
	Name               string `json:"name"`
	Priority           int    `json:"priority"`
	StartTime          string `json:"startTime"`
	Timestamp          int64  `json:"timestamp"`
	Uptime             int64  `json:"uptime"`
	HeartbeatTimestamp int64  `json:"heartbeatTimestamp"`
	HeartbeatInterval  int64  `json:"heartbeatInterval"`
}

type TaskState int

const (
	TaskStateUnknown  = iota
	TaskStateStarting = iota
	TaskStateRunning  = iota
	TaskStateRetrying
	TaskStateStopping
	TaskStateStopped
)

type Task struct {
	mu                sync.RWMutex
	Name              string
	Value             any
	Priority          int
	maxRetry          int
	retryLeft         int
	retryDelay        time.Duration
	watchdogTimeout   time.Duration
	maxRuntime        time.Duration
	startTime         time.Time
	lastHeartbeatTime time.Time
	heartbeatInterval time.Duration
	run               TaskRunFunc
	ctx               context.Context
	cancel            context.CancelFunc
	index             int // 在堆中的索引
	heartbeatCh       chan struct{}
	timer             *time.Timer
	timeoutCh         <-chan time.Time
	doneCh            chan error
	state             TaskState
}

func (t *Task) Keepalive() {
	t.mu.Lock()
	defer t.mu.Unlock()
	select {
	case t.heartbeatCh <- struct{}{}:
		var (
			prev = t.lastHeartbeatTime
			now  = time.Now()
		)
		if prev.UnixMilli() < 0 {
			t.lastHeartbeatTime = now
		}
		t.lastHeartbeatTime = now
		t.heartbeatInterval = now.Sub(prev)
	default:
	}
}

func (t *Task) SetState(state TaskState) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state = state
}

func (t *Task) GetState() TaskState {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.state
}

func (t *Task) Info() TaskInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return TaskInfo{
		Name:               t.Name,
		Priority:           t.Priority,
		StartTime:          t.startTime.Format("2006-01-02 15:04:05"),
		Timestamp:          t.startTime.UnixMilli(),
		Uptime:             time.Since(t.startTime).Milliseconds(),
		HeartbeatTimestamp: t.lastHeartbeatTime.UnixMilli(),
		HeartbeatInterval:  t.heartbeatInterval.Milliseconds(),
	}
}

func (t *Task) SetStartTime(ts time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.startTime = ts
	t.lastHeartbeatTime = ts
}

func (t *Task) GetStartTime() time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.startTime
}

func NewTask(opts ...Option) *Task {
	task := NewDefaultTask()
	for _, opt := range opts {
		opt(task)
	}
	return task
}

func NewDefaultTask() *Task {
	ctx, cancel := context.WithCancel(context.Background())
	return &Task{
		Name:            "default",
		Value:           nil,
		Priority:        100,
		maxRetry:        3,
		retryLeft:       3,
		retryDelay:      3 * time.Second,
		watchdogTimeout: 2 * time.Second,
		run: func(ctx context.Context, task *Task) error {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return nil
				case <-ticker.C:
					task.Keepalive()
				}
			}
		},
		ctx:         ctx,
		cancel:      cancel,
		index:       0,
		heartbeatCh: make(chan struct{}, 1),
		timeoutCh:   make(chan time.Time, 1),
		doneCh:      make(chan error, 1),
		state:       TaskStateUnknown,
	}
}
