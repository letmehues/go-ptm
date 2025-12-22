package ptm

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

type TaskManager struct {
	mu        sync.RWMutex
	tasks     map[string]*Task
	capacity  int
	submitted uint64 // 任务提交计数
	evicted   uint64 // 任务丢弃计数
	failure   uint64 // 任务失败计数

	pq        PriorityQueue
	Callbacks Callbacks

	OnError func(taskId string, err error)

	ctx    context.Context
	cancel context.CancelFunc
}

func NewTaskManager(capacity int) *TaskManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &TaskManager{
		pq:        NewPriorityQueue(capacity),
		tasks:     make(map[string]*Task),
		capacity:  capacity,
		ctx:       ctx,
		cancel:    cancel,
		Callbacks: NewDefaultCallbacks(),
	}
}

func (tm *TaskManager) TaskMap() map[string]*Task {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	copyMap := make(map[string]*Task, len(tm.tasks))
	for _, task := range tm.tasks {
		copyMap[task.Name] = task
	}
	return copyMap
}

func (tm *TaskManager) Submit(task *Task) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.existsInternal(task) {
		return ErrTaskAlreadyExists
	}

	if len(tm.tasks) >= tm.capacity {
		if tm.pq.Len() == 0 {
			return ErrTaskQueueIsEmpty
		}
		lowest := tm.pq[0]
		if task.Priority <= lowest.Priority {
			return ErrTaskQueueIsFull
		}
		_ = tm.stopTaskInternal(lowest)
		atomic.AddUint64(&tm.evicted, 1)
	}
	heap.Push(&tm.pq, task)
	tm.tasks[task.Name] = task
	go tm.runTask(task)
	atomic.AddUint64(&tm.submitted, 1)
	return nil
}

func (tm *TaskManager) runTask(task *Task) {
	var reason error
	tm.Callbacks.OnStart(task.Name)
	defer func() {
		task.SetState(TaskStateStopping)
		err := tm.StopTask(task.Name)
		if reason == nil && err != nil {
			reason = err
		}
		if reason == nil && task.ctx.Err() != nil {
			reason = task.ctx.Err()
		}
		tm.Callbacks.OnStop(task.Name, reason)
		task.SetState(TaskStateStopped)
	}()
	for {
		if task.ctx.Err() != nil {
			return
		}
		task.SetStartTime(time.Now())
		task.SetState(TaskStateStarting)
		err := tm.runTaskWithWatchdog(task)
		if err == nil {
			return
		}
		if errors.Is(err, context.DeadlineExceeded) ||
			errors.Is(err, context.Canceled) {
			reason = err
			return
		}

		if task.retryLeft > 0 {
			task.retryLeft--
			reason = err
			task.SetState(TaskStateRetrying)
			tm.Callbacks.OnRetry(task.Name, task.retryLeft, task.retryDelay, err)
			select {
			case <-time.After(task.retryDelay):
				continue
			case <-task.ctx.Done():
				return
			}
		} else {
			atomic.AddUint64(&tm.failure, 1)
			return
		}
	}
}

//func (tm *TaskManager) UpdateTaskPriority(task *Task) error {
//	tm.mu.Lock()
//	defer tm.mu.Unlock()
//	if !tm.existsInternal(task) {
//		return ErrTaskNotExists
//	}
//	task.UpdatePriority()
//	heap.Fix(&tm.pq, task.index)
//	return nil
//}

//func (tm *TaskManager) Resize(capacity int) error {
//	tm.mu.RLock()
//	defer tm.mu.RUnlock()
//	return nil
//}

func (tm *TaskManager) runTaskWithWatchdog(task *Task) error {
	var (
		runCtx    context.Context
		runCancel context.CancelFunc
	)
	if task.maxRuntime > 0 {
		runCtx, runCancel = context.WithTimeout(task.ctx, task.maxRuntime)
	} else {
		runCtx, runCancel = context.WithCancel(task.ctx)
	}
	defer runCancel()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				task.doneCh <- fmt.Errorf("panic recovered: %v\nStack: %s", r, string(debug.Stack()))
			}
		}()
		task.doneCh <- task.run(runCtx, task)
	}()

	if task.watchdogTimeout > 0 {
		task.timer = time.NewTimer(task.watchdogTimeout)
		defer task.timer.Stop()
		task.timeoutCh = task.timer.C
	}
	for {
		select {
		case err := <-task.doneCh:
			if errors.Is(err, context.Canceled) {
				select {
				case <-task.timeoutCh:
					return ErrTaskHeartbeatTimeout
				default:
				}
			}
			return err
		case <-task.heartbeatCh:
			if task.timer != nil {
				if !task.timer.Stop() {
					select {
					case <-task.timer.C:
					default:
					}
				}
				task.timer.Reset(task.watchdogTimeout)
				//slog.Info("task heartbeat...", slog.Any("task_name", task.Name))
			}
		case <-task.timeoutCh:
			return ErrTaskHeartbeatTimeout
		case <-task.ctx.Done():
			return task.ctx.Err()
		}
	}
}

func (tm *TaskManager) Shutdown() {
	tm.cancel()
}

func (tm *TaskManager) StopTask(name string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.stopTaskInternal(tm.tasks[name])
}

//nolint:unused // 保留用于调试
func (tm *TaskManager) printTasks() {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	stats := tm.Stats()
	slog.Info("task stats",
		slog.Any("running", stats.Running),
		slog.Any("capacity", stats.Capacity),
		slog.Any("submitted", stats.Submitted),
		slog.Any("evicted", stats.Evicted),
		slog.Any("failure", stats.Failure),
	)
}

func (tm *TaskManager) existsInternal(task *Task) bool {
	_, ok := tm.tasks[task.Name]
	return ok
}

func (tm *TaskManager) stopTaskInternal(task *Task) error {
	if task == nil {
		return ErrTaskIsNil
	}
	if !tm.existsInternal(task) {
		return ErrTaskNotExists
	}
	task.cancel()
	if task.index >= 0 {
		heap.Remove(&tm.pq, task.index)
	}
	delete(tm.tasks, task.Name)
	return nil
}

//nolint:unused // 保留用于内部状态检查
func (tm *TaskManager) verifyInvariants() error {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	// 1. 检查 Map 和 Heap 大小是否一致
	if len(tm.tasks) != tm.pq.Len() {
		return fmt.Errorf("map(%d) and heap(%d) size mismatch", tm.pq.Len(), len(tm.tasks))
	}

	// 2. 检查是否超过容量
	if len(tm.tasks) > tm.capacity {
		return fmt.Errorf("capacity breached: %d > %d", len(tm.tasks), tm.capacity)
	}

	// 3. 检查 Heap 的索引是否正确 (这是并发删除最容易出错的地方)
	for i, item := range tm.pq {
		if item.index != i {
			return fmt.Errorf("heap index corruption: task '%s' has index %d, but is at pos %d", item.Name, item.index, i)
		}
		if mapItem, exists := tm.tasks[item.Name]; !exists || mapItem != item {
			return fmt.Errorf("map/heap mismatch for task '%s'", item.Name)
		}
	}
	return nil
}
