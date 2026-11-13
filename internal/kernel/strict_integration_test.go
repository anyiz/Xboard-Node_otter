package kernel_test

import (
	"context"
	"encoding/binary"
	"fmt"
	"github.com/cedar2025/xboard-node/internal/config"
	"github.com/cedar2025/xboard-node/internal/devicelease"
	"github.com/cedar2025/xboard-node/internal/kernel"
	"github.com/cedar2025/xboard-node/internal/kernel/singbox"
	"github.com/cedar2025/xboard-node/internal/kernel/xray"
	"github.com/cedar2025/xboard-node/internal/model"
	"github.com/google/uuid"
	pp "github.com/pires/go-proxyproto"
	"io"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"
)

// The Lua suite tests the authority itself; this test proves both real kernels
// use the transmitted address and cannot pass payload before admission.
type authority struct {
	mu  sync.Mutex
	ip  string
	ttl float64
}

func (a *authority) Call(_ context.Context, op string, items []devicelease.Item) ([]devicelease.Result, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]devicelease.Result, len(items))
	for i, item := range items {
		if op == "release" {
			out[i].Allowed = true
			continue
		}
		if a.ip == "" {
			a.ip = item.IP
		}
		out[i] = devicelease.Result{Allowed: item.IP == a.ip, TTL: a.ttl}
	}
	return out, nil
}
func freePort(t *testing.T) int {
	t.Helper()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	p := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return p
}
func openVLESS(t *testing.T, port int, uid uuid.UUID, source string, target int, proxy int) (net.Conn, error) {
	t.Helper()
	c, e := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), time.Second)
	if e != nil {
		return nil, e
	}
	c.SetDeadline(time.Now().Add(2 * time.Second))
	if proxy > 0 {
		h := pp.HeaderProxyFromAddrs(byte(proxy), &net.TCPAddr{IP: net.ParseIP(source), Port: 12345}, &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: port})
		if _, e = h.WriteTo(c); e != nil {
			c.Close()
			return nil, e
		}
	}
	request := []byte{0}
	request = append(request, uid[:]...)
	request = append(request, 0, 1)
	request = binary.BigEndian.AppendUint16(request, uint16(target))
	request = append(request, 1, 127, 0, 0, 1)
	request = append(request, []byte("ping")...)
	if _, e = c.Write(request); e != nil {
		c.Close()
		return nil, e
	}
	response := make([]byte, 6)
	_, e = io.ReadFull(c, response)
	if e != nil {
		c.Close()
		return nil, e
	}
	if string(response[2:]) != "ping" {
		c.Close()
		return nil, fmt.Errorf("unexpected response %x", response)
	}
	return c, nil
}

func TestStrictGlobalAdmissionRealKernels(t *testing.T) {
	for _, kind := range []string{"singbox", "xray"} {
		t.Run(kind, func(t *testing.T) {
			echo, e := net.Listen("tcp", "127.0.0.1:0")
			if e != nil {
				t.Fatal(e)
			}
			defer echo.Close()
			go func() {
				for {
					c, e := echo.Accept()
					if e != nil {
						return
					}
					go func() { defer c.Close(); io.Copy(c, c) }()
				}
			}()
			target := echo.Addr().(*net.TCPAddr).Port
			id := uuid.New()
			api := &authority{ttl: 6}
			var ports []int
			for i := 0; i < 3; i++ {
				manager := devicelease.New(api)
				t.Cleanup(manager.Close)
				ctx, cancel := context.WithCancel(context.Background())
				t.Cleanup(cancel)
				go manager.Run(ctx)
				cfg := config.KernelConfig{Type: kind, StrictDeviceLeases: true, TrustedProxies: []string{"127.0.0.1/32"}, LeaseManager: manager, LogLevel: "error", ConfigDir: t.TempDir()}
				if i == 2 {
					cfg.TrustedProxies = []string{"192.0.2.0/24"}
				}
				if kind == "singbox" {
					cfg.CustomRoute = []map[string]any{{"ip_cidr": []string{"127.0.0.1/32"}, "outbound": "direct"}}
				} else {
					cfg.CustomRoute = []map[string]any{{"type": "field", "ip": []string{"127.0.0.1/32"}, "outboundTag": "direct"}}
				}
				var k kernel.Kernel
				if kind == "singbox" {
					k = singbox.New(cfg)
				} else {
					k = xray.New(cfg)
				}
				port := freePort(t)
				ports = append(ports, port)
				if e = k.Start(&model.NodeSpec{Protocol: "vless", ListenIP: "127.0.0.1", ServerPort: port, Network: "tcp", StrictDeviceLeases: true}, []model.UserSpec{{ID: 1, UUID: id.String(), DeviceLimit: 1}}, kernel.TLSCert{}); e != nil {
					t.Fatal(e)
				}
				t.Cleanup(k.Stop)
			}
			first, e := openVLESS(t, ports[0], id, "192.0.2.1", target, 2)
			if e != nil {
				t.Fatalf("first IP: %v", e)
			}
			defer first.Close()
			same, e := openVLESS(t, ports[1], id, "192.0.2.1", target, 2)
			if e != nil {
				t.Fatalf("same IP at second node: %v", e)
			}
			defer same.Close()
			v1, e := openVLESS(t, ports[1], id, "192.0.2.1", target, 1)
			if e != nil {
				t.Fatalf("PROXY v1: %v", e)
			}
			defer v1.Close()
			if c, e := openVLESS(t, ports[2], id, "192.0.2.1", target, 2); e == nil {
				c.Close()
				t.Fatal("untrusted sender accepted")
			}

			if c, e := openVLESS(t, ports[1], id, "192.0.2.2", target, 2); e == nil {
				c.Close()
				t.Fatal("different IP bypassed global limit")
			}
			if c, e := openVLESS(t, ports[0], id, "", target, 0); e == nil {
				c.Close()
				t.Fatal("missing PROXY header accepted")
			}
			time.Sleep(1200 * time.Millisecond)
			first.SetDeadline(time.Now().Add(time.Second))
			_, writeErr := first.Write([]byte("late"))
			var data [4]byte
			_, readErr := io.ReadFull(first, data[:])
			if writeErr == nil && readErr == nil {
				t.Fatal("payload passed after lease expiration")
			}
			api.mu.Lock()
			seen := api.ip
			api.mu.Unlock()
			if seen != "192.0.2.1" {
				t.Fatalf("wrong source: %s", seen)
			}
		})
	}
}
