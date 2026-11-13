package devicelease

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"
)

type fakeAPI struct {
	mu       sync.Mutex
	acquires int
	fail     bool
	ttl      float64
}

func (f *fakeAPI) Call(ctx context.Context, op string, items []Item) ([]Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return nil, errors.New("offline")
	}
	if op == "acquire" {
		f.acquires++
	}
	out := make([]Result, len(items))
	for i := range out {
		out[i] = Result{Allowed: true, TTL: f.ttl}
	}
	return out, nil
}
func TestCoalescesConcurrentConnections(t *testing.T) {
	api := &fakeAPI{ttl: 60}
	m := New(api)
	defer m.Close()
	var wg sync.WaitGroup
	handles := make(chan *Handle, 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h, e := m.Acquire(context.Background(), 1, "uuid", "::ffff:192.0.2.1", nil)
			if e != nil {
				t.Error(e)
				return
			}
			handles <- h
		}()
	}
	wg.Wait()
	close(handles)
	api.mu.Lock()
	count := api.acquires
	api.mu.Unlock()
	if count != 1 {
		t.Fatalf("got %d grants", count)
	}
	for h := range handles {
		h.Close()
	}
}
func TestOfflineFailClosedAndExpiredCannotRevive(t *testing.T) {
	api := &fakeAPI{ttl: 60}
	m := New(api)
	defer m.Close()
	closed := make(chan struct{}, 1)
	h, e := m.Acquire(context.Background(), 1, "uuid", "192.0.2.1", func() { closed <- struct{}{} })
	if e != nil {
		t.Fatal(e)
	}
	api.mu.Lock()
	api.fail = true
	api.mu.Unlock()
	if _, e = m.Acquire(context.Background(), 1, "uuid", "192.0.2.2", nil); e == nil {
		t.Fatal("offline acquire allowed")
	}
	if !h.Valid() {
		t.Fatal("unexpired lease lost on network failure")
	}
	m.mu.Lock()
	h.entry.deadline = time.Now().Add(-time.Second)
	m.mu.Unlock()
	m.expire()
	select {
	case <-closed:
	default:
		t.Fatal("expired socket not closed")
	}
	if h.Valid() {
		t.Fatal("expired lease valid")
	}
}
func TestGuardBlocksCachedPayloadAfterExpiry(t *testing.T) {
	api := &fakeAPI{ttl: 60}
	m := New(api)
	defer m.Close()
	a, b := net.Pipe()
	defer b.Close()
	h, e := m.Acquire(context.Background(), 1, "uuid", "192.0.2.1", func() { a.Close() })
	if e != nil {
		t.Fatal(e)
	}
	c := &Conn{Conn: a, Lease: h}
	m.mu.Lock()
	h.entry.deadline = time.Now().Add(-time.Second)
	m.mu.Unlock()
	if _, e = c.Write([]byte("must not pass")); !errors.Is(e, ErrDenied) {
		t.Fatalf("write: %v", e)
	}
	if _, e = c.Read(make([]byte, 1)); !errors.Is(e, ErrDenied) {
		t.Fatalf("read: %v", e)
	}
}
func TestRenewUsesRequestStartAndNeverRevives(t *testing.T) {
	api := &fakeAPI{ttl: 60}
	m := New(api)
	defer m.Close()
	h, e := m.Acquire(context.Background(), 1, "uuid", "192.0.2.1", nil)
	if e != nil {
		t.Fatal(e)
	}
	if time.Until(h.Deadline()) > 55*time.Second {
		t.Fatal("unsafe lease deadline")
	}
	m.mu.Lock()
	h.entry.deadline = time.Now().Add(-time.Second)
	m.mu.Unlock()
	m.renew(context.Background())
	if h.Valid() {
		t.Fatal("renew revived expired handle")
	}
}

func TestGuardPreservesProtocolReadDeadline(t *testing.T) {
	m := New(&fakeAPI{ttl: 60})
	defer m.Close()
	a, b := net.Pipe()
	defer b.Close()
	h, e := m.Acquire(context.Background(), 1, "uuid", "192.0.2.1", func() { a.Close() })
	if e != nil {
		t.Fatal(e)
	}
	c := &Conn{Conn: a, Lease: h}
	defer c.Close()
	c.SetReadDeadline(time.Now().Add(20 * time.Millisecond))
	start := time.Now()
	_, e = c.Read(make([]byte, 1))
	if timeout, ok := e.(net.Error); !ok || !timeout.Timeout() {
		t.Fatalf("expected timeout, got %v", e)
	}
	if time.Since(start) > time.Second {
		t.Fatal("protocol deadline overwritten by lease deadline")
	}
}
