package socket

import (
	"fmt"
	"sync"
)

// counter is a process-wide listener ID sequence used by Tail.
var counter int64

// Job represents a background execution job (e.g. provisioning, deployment,
// rollback) with a replayable log buffer and live listener fan-out.
type Job struct {
	ID        string
	Mu        sync.Mutex
	Logs      []string
	Listeners map[string]chan string
	Done      bool
}

// nextListenerID returns a unique listener identifier.
func (j *Job) nextListenerID() string {
	j.Mu.Lock()
	defer j.Mu.Unlock()
	counter++
	return fmt.Sprintf("tail-%d", counter)
}

// NewJob initializes a Job with an empty listener registry.
func NewJob(id string) *Job {
	return &Job{
		ID:        id,
		Listeners: make(map[string]chan string),
	}
}

// Reset clears the log history and marks the job as running again.
func (j *Job) Reset() {
	j.Mu.Lock()
	j.Logs = nil
	j.Done = false
	j.Mu.Unlock()
}

// AddLog appends a line to the job history and broadcasts it to every
// listener without blocking.
func (j *Job) AddLog(line string) {
	j.Mu.Lock()
	j.Logs = append(j.Logs, line)

	listeners := make([]chan string, 0, len(j.Listeners))
	for _, ch := range j.Listeners {
		listeners = append(listeners, ch)
	}
	j.Mu.Unlock()

	for _, ch := range listeners {
		select {
		case ch <- line:
		default:
		}
	}
}

// GetLogs returns a snapshot copy of the current logs.
func (j *Job) GetLogs() []string {
	j.Mu.Lock()
	defer j.Mu.Unlock()

	logs := make([]string, len(j.Logs))
	copy(logs, j.Logs)
	return logs
}

// RegisterListener subscribes a buffered channel for live log lines.
func (j *Job) RegisterListener(id string) chan string {
	j.Mu.Lock()
	defer j.Mu.Unlock()

	ch := make(chan string, 200)
	j.Listeners[id] = ch
	return ch
}

// UnregisterListener removes and closes a listener channel by ID.
func (j *Job) UnregisterListener(id string) {
	j.Mu.Lock()
	defer j.Mu.Unlock()

	if ch, ok := j.Listeners[id]; ok {
		close(ch)
		delete(j.Listeners, id)
	}
}
