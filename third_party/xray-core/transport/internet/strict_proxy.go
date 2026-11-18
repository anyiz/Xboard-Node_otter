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

type proxyPolicy struct {
	mode     string
	prefixes []netip.Prefix
}

var strictProxyPorts sync.Map // port -> proxyPolicy; configured before instance start

func SetStrictProxy(port int, cidrs []string) error {
	return setProxyPolicy(port, "strict", cidrs)
}

// SetCompatibleProxy trusts PROXY headers from configured relays while still
// accepting ordinary direct clients. A trusted relay must always send a valid
// header, otherwise its own address could be mistaken for the client address.
func SetCompatibleProxy(port int, cidrs []string) error {
	return setProxyPolicy(port, "compatible", cidrs)
}

func ClearProxyPolicy(port int) { strictProxyPorts.Delete(port) }

func setProxyPolicy(port int, mode string, cidrs []string) error {
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
	policy := proxyPolicy{mode: mode, prefixes: prefixes}
	if old, loaded := strictProxyPorts.Load(port); loaded && reflect.DeepEqual(old, policy) {
		return nil
	}
	strictProxyPorts.Store(port, policy)
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
	policy := v.(proxyPolicy)
	return &pp.Listener{Listener: l, ReadHeaderTimeout: 5 * time.Second,
		Policy: func(peer net.Addr) (pp.Policy, error) {
			a, e := netip.ParseAddrPort(peer.String())
			if e != nil {
				return pp.REJECT, e
			}
			for _, p := range policy.prefixes {
				if p.Contains(a.Addr().Unmap()) {
					return pp.REQUIRE, nil
				}
			}
			if policy.mode == "compatible" {
				return pp.SKIP, nil
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
