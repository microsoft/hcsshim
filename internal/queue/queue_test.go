package queue

import (
	"sync"
	"testing"
	"time"
)

func TestEnqueueDequeue(t *testing.T) {
	q := NewMessageQueue()

	vals := []int{1, 2, 3, 4, 5}
	for _, val := range vals {
		// Enqueue vals to the queue and read later.
		if err := q.Enqueue(val); err != nil {
			t.Fatal(err)
		}
	}

	for _, val := range vals {
		// Dequeueing from an empty queue should block forever until a write occurs.
		qVal, err := q.Dequeue()
		if err != nil {
			t.Fatal(err)
		}

		if qVal != val {
			t.Fatalf("expected %d, got: %d", val, qVal)
		}
	}
}

func TestEnqueueDequeueClose(t *testing.T) {
	q := NewMessageQueue()

	vals := []int{1, 2, 3}
	go func() {
		for _, val := range vals {
			_ = q.Enqueue(val)
		}
	}()

	read := 0
	for {
		if _, err := q.Dequeue(); err == nil {
			read++
			if read == len(vals) {
				// Close after we've read all of our values, then on the next
				// go around make sure we get ErrClosed()
				q.Close()
			}
		} else if err != ErrQueueClosed {
			t.Fatalf("expected to receive ErrQueueClosed, instead got: %s", err)
		}
		break
	}
}

func TestMultipleReaders(t *testing.T) {
	q := NewMessageQueue()
	errChan := make(chan error)
	done := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			if err := q.Enqueue(1); err != nil {
				errChan <- err
			}
		}
	}()

	wg := sync.WaitGroup{}
	wg.Add(2)

	// Reader 1
	go func() {
		for i := 0; i < 25; i++ {
			if _, err := q.Dequeue(); err != nil {
				errChan <- err
			}
		}
		wg.Done()
	}()

	// Reader 2
	go func() {
		for i := 0; i < 25; i++ {
			if _, err := q.Dequeue(); err != nil {
				errChan <- err
			}
		}
		wg.Done()
	}()

	go func() {
		wg.Wait()
		done <- struct{}{}
	}()

	select {
	case err := <-errChan:
		t.Fatalf("failed in read or write: %s", err)
	case <-done:
	case <-time.After(time.Second * 20):
		t.Fatalf("timeout exceeded waiting for reads to complete")
	}
}

func TestMultipleReadersClose(t *testing.T) {
	q := NewMessageQueue()
	errChan := make(chan error)
	done := make(chan struct{})

	wg := sync.WaitGroup{}
	wg.Add(2)

	// Reader 1
	go func() {
		if _, err := q.Dequeue(); err != ErrQueueClosed {
			errChan <- err
		}
		wg.Done()
	}()

	// Reader 2
	go func() {
		if _, err := q.Dequeue(); err != ErrQueueClosed {
			errChan <- err
		}
		wg.Done()
	}()

	go func() {
		wg.Wait()
		done <- struct{}{}
	}()

	time.Sleep(time.Second * 2)
	// Close the queue and this should signal both readers to return ErrQueueClosed.
	q.Close()

	select {
	case err := <-errChan:
		t.Fatalf("failed in read or write: %s", err)
	case <-done:
	case <-time.After(time.Second * 20):
		t.Fatalf("timeout exceeded waiting for reads to complete")
	}
}
