package listener

import (
	"bufio"
	"fmt"
	pp "github.com/pires/go-proxyproto"
	"net"
	"net/netip"
	"time"
)

// Parse in each accepted connection's goroutine, never block the accept loop.
func acceptProxy(conn net.Conn, trusted []string, acceptDirect bool) (net.Conn, error) {
	peer, err := netip.ParseAddrPort(conn.RemoteAddr().String())
	if err != nil {
		return nil, err
	}
	allowed := false
	for _, cidr := range trusted {
		p, e := netip.ParsePrefix(cidr)
		if e != nil {
			return nil, e
		}
		if p.Contains(peer.Addr().Unmap()) {
			allowed = true
		}
	}
	if !allowed {
		if acceptDirect {
			return conn, nil
		}
		return nil, fmt.Errorf("untrusted PROXY peer")
	}
	if err = conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return nil, err
	}
	reader := bufio.NewReader(conn)
	header, err := pp.Read(reader)
	if err != nil {
		return nil, err
	}
	if header == nil || header.Command != pp.PROXY {
		return nil, fmt.Errorf("PROXY source required")
	}
	source, dest, ok := header.TCPAddrs()
	if !ok || source.IP.IsUnspecified() {
		return nil, fmt.Errorf("TCP source required")
	}
	if err = conn.SetReadDeadline(time.Time{}); err != nil {
		return nil, err
	}
	return &proxySourceConn{Conn: conn, reader: reader, source: source, dest: dest}, nil
}

type proxySourceConn struct {
	net.Conn
	reader       *bufio.Reader
	source, dest net.Addr
}

func (c *proxySourceConn) Read(p []byte) (int, error) { return c.reader.Read(p) }
func (c *proxySourceConn) RemoteAddr() net.Addr       { return c.source }
func (c *proxySourceConn) LocalAddr() net.Addr        { return c.dest }
