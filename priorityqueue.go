package ptm

type PriorityQueue []*Task

func NewPriorityQueue(cap int) PriorityQueue {
	return make(PriorityQueue, 0, cap)
}

func (pq PriorityQueue) Len() int { return len(pq) }

// Less 决定排序规则。这里使用 < 号，建立最小堆
// pq[i].Priority < pq[j].Priority 表示堆顶（Index 0）永远是优先级最低的任务
func (pq PriorityQueue) Less(i, j int) bool {
	if pq[i].Priority != pq[j].Priority {
		return pq[i].Priority < pq[j].Priority
	}
	return pq[i].GetStartTime().Before(pq[j].GetStartTime())
}

func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *PriorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*Task)
	item.index = n
	*pq = append(*pq, item)
}

func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // 避免内存泄漏
	item.index = -1 // 标记为已移除
	*pq = old[0 : n-1]
	return item
}
