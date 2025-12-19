package ptm

import (
	"errors"
)

var (
	ErrTaskIsNil            = errors.New("task is nil")
	ErrTaskAlreadyExists    = errors.New("task already exists")
	ErrTaskNotExists        = errors.New("task not exists")
	ErrTaskQueueIsFull      = errors.New("task queue is full")
	ErrTaskQueueIsEmpty     = errors.New("task queue is empty")
	ErrTaskHeartbeatTimeout = errors.New("task heartbeat watchdogTimeout")
)
