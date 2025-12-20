# Go-PTM

Go-PTM is a robust, in-process task supervisor designed for managing long-running, stateful, and blocking tasks.
Unlike traditional worker pools that focus on throughput for short-lived jobs, Go-PTM focuses on reliability and
lifecycle management. It is ideal for scenarios like RTSP video stream analysis, AI model inference, or IoT device
connections where tasks must be kept alive, monitored for deadlocks, and managed based on priority when resources are
constrained.

## ✨ Key Features

- 🛡️ Supervisor Pattern: Automatically restarts tasks upon failure (error) or panic, with configurable retry limits and
  backoff strategies.
- 🐶 Watchdog Mechanism: Detects "frozen" or deadlocked tasks via heartbeat monitoring. If a task fails to report a
  heartbeat, it is killed and restarted.
- ⚖️ Priority Eviction: When the manager reaches capacity, lower-priority tasks are automatically evicted (stopped) to
  make room for higher-priority ones.
- ⏱️ Lifecycle Control: Supports MaxRuntime (auto-stop after a duration) and context-based graceful cancellation.
- 🚀 Concurrency Safe: Fully thread-safe operations for adding, stopping, and managing tasks.

## 📦 Installation

```bash
go get github.com/your-username/go-sentinel
```

## 🚀 Quick Start

```go
package main

import (
	"context"
	"fmt"
	"time"
	"github.com/letmehues/go-ptm"
)

func main() {
	// Create a manager with a capacity of 10 tasks
	manager := ptm.NewTaskManager(10)
	defer manager.Shutdown()

	process := func(ctx context.Context, task *ptm.Task) error {
		fmt.Println("Task started...")

		// Simulate a blocking loop (e.g., reading video frames)
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				// Handle cancellation gracefully
				return ctx.Err()
			case <-ticker.C:
				// Perform business logic
				fmt.Println("Processing frame...")

				// Report heartbeat to watchdog
				task.Keepalive()
			}

		}
	}

	// Define task 
	task := ptm.NewTask(
		ptm.WithName("demo_task"),
		ptm.WithPriority(100),              // Higher number = Higher priority
		ptm.WithMaxRetry(3, 2*time.Second), // Retry up to 3 times on failure, Wait 2s before retrying
		ptm.WithTimeout(5*time.Second),     // Kill if no heartbeat for 5s
		ptm.WithRunFunc(process),
	)

	// Add and start the task
	err := manager.Submit(task)
	if err != nil {
		panic(err)
	}
	// Keep main process running
	time.Sleep(30 * time.Second)
}
```

## ⚙️ Core Concepts

### Priority Eviction

When you try to add a task but the manager is full (len >= capacity), Go-PTM checks the Priority Queue.

- If the new task's priority > the lowest priority in the pool: The lowest task is stopped (evicted), and the new task
  is added.
- Otherwise: Returns an error immediately.

### Watchdog & Heartbeat

For blocking tasks (like reading network streams), relying solely on error return is not enough. The task might hang
forever.

- Go-PTM has a keepAlive function into your task.
- You must call keepAlive() periodically.
- If the time since the last heartbeat exceeds WatchdogTimeout, the task context is cancelled, and it is treated as a
  failure (triggering retry).

## 🤝 Contributing

Pull requests are welcome! For major changes, please open an issue first to discuss what you would like to change.

## 📄 License

MIT