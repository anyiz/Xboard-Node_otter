package devicelease

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"
)

var ErrDenied = errors.New("global IP lease denied or expired")

type entry struct {
	item     Item
	ready    chan struct{}
	deadline time.Time
	handles  map[*Handle]struct{}
}
type Manager struct {
	api      API
	instance string
	mu       sync.Mutex
	entries  map[string]*entry
	closed   bool
	renewing atomic.Bool
	denied   atomic.Uint64
	expired  atomic.Uint64
}
type Handle struct {
	manager    *Manager
	key        string
	entry      *entry
	onExpire   func()
	once       sync.Once
	expireOnce sync.Once
}

func New(api API) *Manager {
	return &Manager{api: api, instance: uuid.NewString(), entries: make(map[string]*entry)}
}
func NormalizeIP(ip string) (string, error) {
	a, e := netip.ParseAddr(ip)
	if e != nil {
		return "", e
	}
	return a.Unmap().String(), nil
}

// Acquire coalesces concurrent first connections; the global decision is always made by the panel.
func (m *Manager) Acquire(ctx context.Context, id int, credential, ip string, expire func()) (*Handle, error) {
	ip, err := NormalizeIP(ip)
	if err != nil || id <= 0 || credential == "" {
		return nil, ErrDenied
	}
	key := credential + "/" + ip
	for {
		m.mu.Lock()
		if m.closed {
			m.mu.Unlock()
			return nil, ErrDenied
		}
		if e := m.entries[key]; e != nil {
			if e.ready != nil {
				ready := e.ready
				m.mu.Unlock()
				select {
				case <-ready:
					continue
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			if !beforeDeadline(time.Now(), e.deadline) {
				m.mu.Unlock()
				return nil, ErrDenied
			}
			h := &Handle{manager: m, key: key, entry: e, onExpire: expire}
			e.handles[h] = struct{}{}
			m.mu.Unlock()
			return h, nil
		}
		e := &entry{item: Item{id, credential, ip, m.instance, uuid.NewString()}, ready: make(chan struct{}), handles: make(map[*Handle]struct{})}
		m.entries[key] = e
		m.mu.Unlock()
		start := time.Now()
		results, callErr := m.api.Call(ctx, "acquire", []Item{e.item})
		m.mu.Lock()
		close(e.ready)
		e.ready = nil
		if callErr != nil || len(results) != 1 || !results[0].Allowed || results[0].TTL <= 5 || m.closed {
			m.denied.Add(1)
			delete(m.entries, key)
			m.mu.Unlock()
			return nil, ErrDenied
		}
		ttl := time.Duration(results[0].TTL * float64(time.Second))
		if ttl > 60*time.Second {
			ttl = 60 * time.Second
		}
		e.deadline = start.Add(ttl - 5*time.Second)
		if !beforeDeadline(time.Now(), e.deadline) {
			delete(m.entries, key)
			m.mu.Unlock()
			return nil, ErrDenied
		}
		h := &Handle{manager: m, key: key, entry: e, onExpire: expire}
		e.handles[h] = struct{}{}
		m.mu.Unlock()
		return h, nil
	}
}
func (h *Handle) Valid() bool {
	h.manager.mu.Lock()
	defer h.manager.mu.Unlock()
	_, ok := h.entry.handles[h]
	return ok && !h.manager.closed && beforeDeadline(time.Now(), h.entry.deadline)
}
func (h *Handle) Deadline() time.Time {
	h.manager.mu.Lock()
	defer h.manager.mu.Unlock()
	return h.entry.deadline
}
func (h *Handle) Close() {
	h.once.Do(func() {
		m := h.manager
		m.mu.Lock()
		delete(h.entry.handles, h)
		last := len(h.entry.handles) == 0
		if last && m.entries[h.key] == h.entry {
			delete(m.entries, h.key)
		}
		m.mu.Unlock()
		if last {
			go func() { _, _ = m.api.Call(context.Background(), "release", []Item{h.entry.item}) }()
		}
	})
}
func (m *Manager) Run(ctx context.Context) {
	renew := time.NewTicker(15 * time.Second)
	sweep := time.NewTicker(250 * time.Millisecond)
	defer renew.Stop()
	defer sweep.Stop()
	defer m.Close()
	for {
		select {
		case <-ctx.Done():
			return
		case <-sweep.C:
			m.expire()
		case <-renew.C:
			go m.renew(ctx)
		}
	}
}
func (m *Manager) expire() {
	var dead []*Handle
	m.mu.Lock()
	for _, e := range m.entries {
		if e.ready == nil && !beforeDeadline(time.Now(), e.deadline) {
			for h := range e.handles {
				dead = append(dead, h)
			}
		}
	}
	m.mu.Unlock()
	for _, h := range dead {
		h.expireOnce.Do(func() {
			m.expired.Add(1)
			if h.onExpire != nil {
				h.onExpire()
			}
		})
		h.Close()
	}
}
func (m *Manager) renew(ctx context.Context) {
	if !m.renewing.CompareAndSwap(false, true) {
		return
	}
	defer m.renewing.Store(false)
	var entries []*entry
	m.mu.Lock()
	for _, e := range m.entries {
		if e.ready == nil && beforeDeadline(time.Now(), e.deadline) {
			entries = append(entries, e)
		}
	}
	m.mu.Unlock()
	for offset := 0; offset < len(entries); offset += 200 {
		end := offset + 200
		if end > len(entries) {
			end = len(entries)
		}
		batch := entries[offset:end]
		items := make([]Item, len(batch))
		for i, e := range batch {
			items[i] = e.item
		}
		start := time.Now()
		results, err := m.api.Call(ctx, "renew", items)
		if err != nil || len(results) != len(batch) {
			continue
		}
		m.mu.Lock()
		for i, e := range batch {
			// Never revive a locally expired lease, even if a delayed response grants it.
			if !beforeDeadline(time.Now(), e.deadline) {
				continue
			}
			r := results[i]
			if !r.Allowed {
				e.deadline = time.Time{}
				continue
			}
			ttl := time.Duration(r.TTL * float64(time.Second))
			if ttl > 60*time.Second {
				ttl = 60 * time.Second
			}
			e.deadline = start.Add(ttl - 5*time.Second)
		}
		m.mu.Unlock()
		m.expire()
	}
}

func (m *Manager) Metrics() map[string]any {
	m.mu.Lock()
	active := 0
	for _, e := range m.entries {
		if e.ready == nil && beforeDeadline(time.Now(), e.deadline) {
			active++
		}
	}
	m.mu.Unlock()
	return map[string]any{"active_ip_leases": active, "denied": m.denied.Load(), "expired_connections": m.expired.Load()}
}
func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	for _, e := range m.entries {
		e.deadline = time.Time{}
	}
	m.mu.Unlock()
	m.expire()
}

// Conn intentionally exposes no unwrap/splice interfaces. Every I/O is authorized.
type Conn struct {
	net.Conn
	Lease                       *Handle
	deadlineMu                  sync.Mutex
	readDeadline, writeDeadline time.Time
}

func earlierDeadline(requested, lease time.Time) time.Time {
	if !requested.IsZero() && requested.Before(lease) {
		return requested
	}
	return lease
}
func (c *Conn) SetReadDeadline(t time.Time) error {
	c.deadlineMu.Lock()
	defer c.deadlineMu.Unlock()
	c.readDeadline = t
	return c.Conn.SetReadDeadline(earlierDeadline(t, c.Lease.Deadline()))
}
func (c *Conn) SetWriteDeadline(t time.Time) error {
	c.deadlineMu.Lock()
	defer c.deadlineMu.Unlock()
	c.writeDeadline = t
	return c.Conn.SetWriteDeadline(earlierDeadline(t, c.Lease.Deadline()))
}
func (c *Conn) SetDeadline(t time.Time) error {
	if err := c.SetReadDeadline(t); err != nil {
		return err
	}
	return c.SetWriteDeadline(t)
}

// Guard also covers buffers inserted by protocol routing after admission.
func (c *Conn) Guard(inner net.Conn) net.Conn { return &Conn{Conn: inner, Lease: c.Lease} }

func (c *Conn) Read(b []byte) (int, error) {
	for {
		if !c.Lease.Valid() {
			c.Close()
			return 0, ErrDenied
		}
		c.deadlineMu.Lock()
		c.Conn.SetReadDeadline(earlierDeadline(c.readDeadline, c.Lease.Deadline()))
		c.deadlineMu.Unlock()
		n, e := c.Conn.Read(b)
		if !c.Lease.Valid() {
			c.Close()
			return 0, ErrDenied
		}
		// A renewal may have extended the lease while this read was blocked.
		if timeout, ok := e.(net.Error); n == 0 && ok && timeout.Timeout() && c.Lease.Valid() {
			c.deadlineMu.Lock()
			requested := c.readDeadline
			c.deadlineMu.Unlock()
			if requested.IsZero() || time.Now().Before(requested) {
				continue
			}
		}
		return n, e
	}
}
func (c *Conn) Write(b []byte) (int, error) {
	if !c.Lease.Valid() {
		c.Close()
		return 0, ErrDenied
	}
	c.deadlineMu.Lock()
	c.Conn.SetWriteDeadline(earlierDeadline(c.writeDeadline, c.Lease.Deadline()))
	c.deadlineMu.Unlock()
	return c.Conn.Write(b)
}
func (c *Conn) Close() error { err := c.Conn.Close(); c.Lease.Close(); return err }

func beforeDeadline(now, deadline time.Time) bool {
	return now.Before(deadline) && now.UnixNano() < deadline.UnixNano()
}
