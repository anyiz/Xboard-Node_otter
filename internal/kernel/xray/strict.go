package xray

import (
	"context"
	"github.com/cedar2025/xboard-node/internal/devicelease"
	"github.com/xtls/xray-core/common"
	"github.com/xtls/xray-core/common/buf"
	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/session"
	"github.com/xtls/xray-core/transport"
	"sync"
)

func (d *LimitDispatcher) authorizeStrict(ctx context.Context, dest net.Destination) (*devicelease.Handle, error) {
	if d.leases == nil {
		return nil, nil
	}
	si := session.InboundFromContext(ctx)
	if si == nil || si.User == nil || si.Conn == nil || dest.Network != net.Network_TCP || (dest.Address.Family().IsDomain() && dest.Address.Domain() == "v1.mux.cool") {
		return nil, devicelease.ErrDenied
	}
	si.CanSpliceCopy = 3
	d.mu.RLock()
	uid := d.emailToUID[si.User.Email]
	credential := d.credentials[si.User.Email]
	d.mu.RUnlock()
	return d.leases.Acquire(ctx, uid, credential, si.Source.Address.IP().String(), func() { si.Conn.Close() })
}
func guardLink(link *transport.Link, h *devicelease.Handle) {
	if h == nil {
		return
	}
	var once sync.Once
	// Capture original streams, avoiding recursion through the wrappers.
	reader, writer := link.Reader, link.Writer
	closeAll := func() { once.Do(func() { common.Interrupt(reader); common.Interrupt(writer); h.Close() }) }
	link.Reader = &leaseReader{Reader: reader, lease: h, close: closeAll}
	link.Writer = &leaseWriter{Writer: writer, lease: h, close: closeAll}
}

type leaseReader struct {
	buf.Reader
	lease *devicelease.Handle
	close func()
}

func (r *leaseReader) ReadMultiBuffer() (buf.MultiBuffer, error) {
	if !r.lease.Valid() {
		r.close()
		return nil, devicelease.ErrDenied
	}
	b, e := r.Reader.ReadMultiBuffer()
	if !r.lease.Valid() {
		buf.ReleaseMulti(b)
		r.close()
		return nil, devicelease.ErrDenied
	}
	if e != nil {
		r.close()
	}
	return b, e
}
func (r *leaseReader) Interrupt()   { r.close() }
func (r *leaseReader) Close() error { r.close(); return nil }

type leaseWriter struct {
	buf.Writer
	lease *devicelease.Handle
	close func()
}

func (w *leaseWriter) WriteMultiBuffer(b buf.MultiBuffer) error {
	if !w.lease.Valid() {
		buf.ReleaseMulti(b)
		w.close()
		return devicelease.ErrDenied
	}
	e := w.Writer.WriteMultiBuffer(b)
	if e != nil {
		w.close()
	}
	return e
}
func (w *leaseWriter) Interrupt()   { w.close() }
func (w *leaseWriter) Close() error { w.close(); return nil }
