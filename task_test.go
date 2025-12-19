package ptm

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"
)

func TestTaskManager_Shutdown(t *testing.T) {
	tm := NewTaskManager(10)
	defer tm.Shutdown()

	for i := 0; i < 100; i++ {
		err := tm.Submit(NewTask(
			WithName(fmt.Sprintf("cam-%d", i)),
			WithValue(i),
			WithPriority(20+10*i),
			WithContext(tm.ctx),
		))
		if err != nil {
			t.Fatal(err)
		}
	}
	cnt := 0
	for cnt < 5 {
		tm.printTasks()
		time.Sleep(100 * time.Millisecond)
		cnt++
	}

}

func TestTaskManager_StopTask(t *testing.T) {
	tm := NewTaskManager(6)
	for i := 0; i < 6; i++ {
		err := tm.Submit(NewTask(
			WithName(fmt.Sprintf("cam-%d", i)),
			WithValue(i),
			WithContext(tm.ctx),
		))
		if err != nil {
			t.Fatal(err)
		}
	}
	go func() {
		for i := 0; i < 6; i++ {
			time.Sleep(100 * time.Millisecond)
			err := tm.StopTask(fmt.Sprintf("cam-%d", i))
			if err == nil {
				tm.printTasks()
			}

		}
	}()
	time.Sleep(1 * time.Second)
}

func TestTaskManager_Submit(t *testing.T) {
	tm := NewTaskManager(6)
	defer tm.Shutdown()

	for i := 0; i < 6; i++ {
		err := tm.Submit(NewTask(
			WithName(fmt.Sprintf("cam-%d", i)),
			WithPriority(50),
			WithValue(i),
			WithContext(tm.ctx),
		))
		if err != nil {
			t.Fatal(err)
		}
	}
	tm.printTasks()
	err := tm.Submit(NewTask(
		WithName("cam-6"),
		WithPriority(10),
		WithValue(6),
		WithContext(tm.ctx),
	))
	if !errors.Is(err, ErrTaskQueueIsFull) {
		t.Fatal(err)
	}
	err = tm.Submit(NewTask(
		WithName("cam-7"),
		WithPriority(90),
		WithValue(7),
		WithContext(tm.ctx),
	))
	if err != nil {
		t.Fatal(err)
	}
	tm.printTasks()

	_ = tm.StopTask("cam-3")
	tm.printTasks()
}

func TestConcurrentSafety(t *testing.T) {
	capacity := 50
	manager := NewTaskManager(capacity)

	workerCount := 20
	opsPerWorker := 1000

	var wg sync.WaitGroup
	wg.Add(workerCount)

	for i := 0; i < workerCount; i++ {
		go func(workerID int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID)))
			for j := 0; j < opsPerWorker; j++ {
				id := r.Intn(capacity * 2)
				name := fmt.Sprintf("task_%02d", id)
				priority := r.Intn(100)
				action := r.Intn(10) // 0-10
				if action < 7 {
					_ = manager.Submit(NewTask(WithName(name), WithPriority(priority)))
				} else {
					_ = manager.StopTask(name)
				}
				if j%100 == 0 {
					time.Sleep(1 * time.Millisecond)
				}
			}
		}(i)
	}

	wg.Wait()

	err := manager.verifyInvariants()
	if err != nil {
		t.Fatal(err)
	}

	manager.mu.Lock()
	defer manager.mu.Unlock()
	tempPQ := make(PriorityQueue, len(manager.pq))
	copy(tempPQ, manager.pq)
	heap.Init(&tempPQ)
	t.Logf("Test finished. Final tasks: %d/%d", len(manager.tasks), capacity)
}

func TestEvictionLogic(t *testing.T) {
	tm := NewTaskManager(3)
	// 填满 3 个低优先级任务
	for i := 0; i < 3; i++ {
		_ = tm.Submit(NewTask(
			WithName(fmt.Sprintf("low_%d", i+1)),
			WithPriority(10),
			WithValue(i+1),
			WithContext(tm.ctx),
			WithRunFunc(func(c context.Context, task *Task) error {
				<-c.Done()
				return nil
			}),
		))
	}
	// 添加高优先级，应该挤掉一个
	err := tm.Submit(NewTask(
		WithName("high_1"),
		WithPriority(100),
		WithValue(1),
		WithContext(tm.ctx),
		WithRunFunc(func(c context.Context, task *Task) error {
			<-c.Done()
			return nil
		}),
	))
	if err != nil {
		t.Fatal(err)
	}

	tm.mu.RLock()
	if len(tm.tasks) != 3 {
		t.Fatal("Expected 3 tasks, got", len(tm.tasks))
	}
	tm.printTasks()
	tm.mu.RUnlock()
}

func TestOnError(t *testing.T) {
	tm := NewTaskManager(10)
	defer tm.Shutdown()
	_ = tm.Submit(NewTask(
		WithName("unstable"),
		WithPriority(100),
		WithTimeout(200*time.Millisecond),
		WithMaxRetry(3, 100*time.Millisecond),
		WithValue(1),
		WithContext(tm.ctx),
		WithRunFunc(func(c context.Context, task *Task) error {
			time.Sleep(50 * time.Millisecond)
			panic("connect watchdogTimeout")
		}),
	))

	_ = tm.Submit(NewTask(
		WithName("error"),
		WithPriority(90),
		WithTimeout(200*time.Millisecond),
		WithMaxRetry(3, 100*time.Millisecond),
		WithValue(2),
		WithContext(tm.ctx),
		WithRunFunc(func(c context.Context, task *Task) error {
			select {
			case <-c.Done():
				return c.Err()
			case <-time.After(20 * time.Millisecond):
				return fmt.Errorf("connnect lost")
			}
		}),
	))

	for {
		time.Sleep(1 * time.Second)
		tm.printTasks()
		if tm.Stats().Running == 0 {
			break
		}
	}
}

func TestWatchdog(t *testing.T) {
	tm := NewTaskManager(10)
	defer tm.Shutdown()

	task := NewTask(
		WithName("test"),
		WithContext(tm.ctx),
		WithMaxRetry(3, 100*time.Millisecond),
		WithTimeout(200*time.Millisecond),
		WithRunFunc(func(ctx context.Context, task *Task) error {
			for i := 0; i < 5; i++ {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				time.Sleep(50 * time.Millisecond)
				fmt.Printf("processed frame %d\n", i)
				if i == 2 {
					fmt.Println("!!! Simulating freeze(stuck for 0.5s)")
					time.Sleep(500 * time.Millisecond)
				}
				task.Keepalive()
			}
			return nil
		}),
	)
	_ = tm.Submit(task)

	for {
		time.Sleep(1 * time.Second)
		tm.printTasks()
		if tm.Stats().Running == 0 {
			break
		}
	}
	tm.printTasks()
}
func TestTaskMaxRuntime(t *testing.T) {
	tm := NewTaskManager(10)
	task := NewTask(
		WithName("test"),
		WithMaxRuntime(300*time.Millisecond),
		WithContext(tm.ctx),
	)
	_ = tm.Submit(task)

	for {
		time.Sleep(100 * time.Millisecond)
		tm.printTasks()
		if tm.Stats().Running == 0 {
			break
		}
	}
}
