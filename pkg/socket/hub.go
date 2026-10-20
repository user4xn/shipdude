package socket

import "sync"

// Hub coordinates active background execution jobs keyed by job ID.
type Hub struct {
	mu   sync.Mutex
	jobs map[string]*Job
}

// NewHub initializes an empty Hub.
func NewHub() *Hub {
	return &Hub{
		jobs: make(map[string]*Job),
	}
}

// GetOrCreateJob returns the job registered under id, creating it on demand.
func (h *Hub) GetOrCreateJob(id string) *Job {
	h.mu.Lock()
	defer h.mu.Unlock()

	if job, ok := h.jobs[id]; ok {
		return job
	}

	job := NewJob(id)
	h.jobs[id] = job
	return job
}

// GetJob returns a job if it exists, otherwise nil.
func (h *Hub) GetJob(id string) *Job {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.jobs[id]
}
