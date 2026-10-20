package socket

// WriterFunc writes a single log line to a stream consumer (e.g. WebSocket).
type WriterFunc func(line string) error

// ReadFunc blocks until the consumer signals it is done (or blocks forever for
// a write-only stream). Returning an error stops the tail.
type ReadFunc func() error

// Tail forwards live log lines of a job to a writer until the writer fails or
// the reader signals completion. When read is nil the call blocks until the
// writer fails. Callers own the underlying connection: closing it unblocks a
// read that Tail leaves behind.
func Tail(job *Job, write WriterFunc, read ReadFunc) error {
	listenerID := job.nextListenerID()
	ch := job.RegisterListener(listenerID)
	defer job.UnregisterListener(listenerID)

	if read == nil {
		for line := range ch {
			if err := write(line); err != nil {
				return err
			}
		}
		return nil
	}

	done := make(chan struct{})
	go func() {
		if read() == nil {
			close(done)
		}
	}()

	for {
		select {
		case <-done:
			return nil
		case line, ok := <-ch:
			if !ok {
				return nil
			}
			if err := write(line); err != nil {
				return err
			}
		}
	}
}
