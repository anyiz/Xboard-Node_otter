package singbox

import (
	"context"
	"github.com/cedar2025/xboard-node/internal/devicelease"
	"github.com/sagernet/sing-box/adapter"
	"net"
)

func (t *ConnTracker) AuthorizeConnection(ctx context.Context, conn net.Conn, metadata adapter.InboundContext) (net.Conn, error) {
	if t.leases == nil {
		return conn, nil
	}
	t.usersMu.RLock()
	uid := t.uuidMap[metadata.User]
	t.usersMu.RUnlock()
	lease, err := t.leases.Acquire(ctx, uid, metadata.User, metadata.Source.Addr.String(), func() { conn.Close() })
	if err != nil {
		conn.Close()
		return nil, err
	}
	return &devicelease.Conn{Conn: conn, Lease: lease}, nil
}
func (t *ConnTracker) RejectPackets() bool { return t.leases != nil }
