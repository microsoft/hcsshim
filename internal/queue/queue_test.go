package queue

import (
	"sync"
	"testing"
	"time"
)

func TestReadWrite(t *testing.T) {
	q := NewMessageQueue()

	// Reading from an empty queue should return ErrQueueEmpty
	if _, err := q.Read(); err != ErrQueueEmpty {
		t.Fatal("expected to receive `ErrQueueEmpty` for reading from empty queue")
	}

	// Write 1 to the queue and read this later.
	if err := q.Write(1); err != nil {
		t.Fatal(err)
	}

	// Read the value. Value will be dequeued.
	if msg, err := q.Read(); err != nil || msg != 1 {
		t.Fatal(err)
	}

	// We just read a value, now try and read again and verify that we get ErrQueueEmpty again.
	if _, err := q.Read(); err != ErrQueueEmpty {
		t.Fatal(err)
	}

	// Close the queue and verify that we get an error on write.
	q.Close()
	if err := q.Write(1); err != ErrQueueClosed {
		t.Fatal(err)
	}
}

func TestReadOrWaitClose(t *testing.T) {
	q := NewMessageQueue()

	go func() {
		q.Write(1)
		q.Write(2)
		q.Write(3)
		time.Sleep(time.Second * 5)
		q.Close()
	}()

	time.Sleep(time.Second * 2)

	read := 0
	for {
		if _, err := q.ReadOrWait(); err != nil {
			if err == ErrQueueClosed && read == 3 {
				break
			}
			t.Fatal(err)
		}
		read++
	}
}

func TestReadOrWait(t *testing.T) {
	q := NewMessageQueue()

	go func() {
		q.Write(1)
		q.Write(2)
		q.Write(3)
		time.Sleep(time.Second * 5)
		q.Write(4)
	}()

	// Small sleep so that we can give time to ensure a value is written to the queue so we
	// can test both states ReadOrWait could be in. These states being there is already a value
	// ready for consumption and all we have to do is just read it, or we wait to get signalled of
	// an available value.
	time.Sleep(time.Second * 1)
	timeout := time.After(time.Second * 20)
	done := make(chan struct{})
	readErr := make(chan error)

	go func() {
		for {
			if msg, err := q.ReadOrWait(); err != nil {
				readErr <- err
			} else {
				if msg == 4 {
					done <- struct{}{}
					break
				}
			}
		}
	}()

	select {
	case <-timeout:
		t.Fatal("timed out waiting for all queue values to be read")
	case <-done:
	case err := <-readErr:
		t.Fatal(err)
	}
}

func TestMultipleReaders(t *testing.T) {
	q := NewMessageQueue()
	errChan := make(chan error)
	done := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			if err := q.Write(1); err != nil {
				errChan <- err
			}
		}
	}()

	wg := sync.WaitGroup{}
	wg.Add(2)

	// Reader 1
	go func() {
		for i := 0; i < 25; i++ {
			if _, err := q.ReadOrWait(); err != nil {
				errChan <- err
			}
		}
		wg.Done()
	}()

	// Reader 2
	go func() {
		for i := 0; i < 25; i++ {
			if _, err := q.ReadOrWait(); err != nil {
				errChan <- err
			}
		}
		wg.Done()
	}()

	go func() {
		wg.Wait()
		done <- struct{}{}
	}()

	timeout := time.After(time.Second * 20)

	select {
	case err := <-errChan:
		t.Fatalf("failed in read or write: %s", err)
	case <-done:
	case <-timeout:
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
		if _, err := q.ReadOrWait(); err != ErrQueueClosed {
			errChan <- err
		}
		wg.Done()
	}()

	// Reader 2
	go func() {
		if _, err := q.ReadOrWait(); err != ErrQueueClosed {
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

	timeout := time.After(time.Second * 20)

	select {
	case err := <-errChan:
		t.Fatalf("failed in read or write: %s", err)
	case <-done:
	case <-timeout:
		t.Fatalf("timeout exceeded waiting for reads to complete")
	}
}
