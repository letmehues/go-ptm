package ptm

import (
	"sort"
	"sync/atomic"
)

type Stats struct {
	Running   int        `json:"running"`
	Capacity  int        `json:"capacity"`
	Submitted uint64     `json:"submitted"`
	Evicted   uint64     `json:"evicted"`
	Failure   uint64     `json:"failure"`
	Tasks     []TaskInfo `json:"tasks"`
}

func (tm *TaskManager) Stats() Stats {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	tasks := make([]TaskInfo, 0, len(tm.tasks))

	for _, task := range tm.pq {
		tasks = append(tasks, task.Info())
	}
	sort.SliceStable(tasks, func(i, j int) bool {
		return tasks[i].Priority > tasks[j].Priority
	})
	return Stats{
		Running:   len(tm.tasks),
		Capacity:  tm.cap,
		Submitted: atomic.LoadUint64(&tm.submitted),
		Evicted:   atomic.LoadUint64(&tm.evicted),
		Failure:   atomic.LoadUint64(&tm.failure),
		Tasks:     tasks,
	}
}
