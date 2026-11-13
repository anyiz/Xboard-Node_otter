package internet

import (
	"fmt"
	pp "github.com/pires/go-proxyproto"
	"net"
	"net/netip"
	"reflect"
	"sync"
	"time"
)

var strictProxyPorts sync.Map // port -> immutable trusted CIDRs; configured before instance start

func SetStrictProxy(port int, cidrs []string) error {
	prefixes := make([]netip.Prefix, 0, len(cidrs))
	for _, s := range cidrs {
		p, e := netip.ParsePrefix(s)
		if e != nil {
			return e
		}
		prefixes = append(prefixes, p)
	}
	if len(prefixes) == 0 {
		return fmt.Errorf("strict PROXY requires trusted sources")
	}
	// Multiple logical nodes must not silently replace each other's trust policy.
	if old, loaded := strictProxyPorts.LoadOrStore(port, prefixes); loaded && !reflect.DeepEqual(old, prefixes) {
		return fmt.Errorf("conflicting strict PROXY trust policy on port %d; restart with distinct ports", port)
	}
	return nil
}

func wrapStrictProxy(l net.Listener) (net.Listener, bool) {
	addr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		return l, false
	}
	v, ok := strictProxyPorts.Load(addr.Port)
	if !ok {
		return l, false
	}
	prefixes := v.([]netip.Prefix)
	return &pp.Listener{Listener: l, ReadHeaderTimeout: 5 * time.Second,
		Policy: func(peer net.Addr) (pp.Policy, error) {
			a, e := netip.ParseAddrPort(peer.String())
			if e != nil {
				return pp.REJECT, e
			}
			for _, p := range prefixes {
				if p.Contains(a.Addr().Unmap()) {
					return pp.REQUIRE, nil
				}
			}
			return pp.REJECT, pp.ErrInvalidUpstream
		},
		ValidateHeader: func(h *pp.Header) error {
			src, _, ok := h.TCPAddrs()
			if h.Command != pp.PROXY || !ok || src.IP.IsUnspecified() {
				return fmt.Errorf("TCP PROXY source required")
			}
			return nil
		},
	}, true
}
