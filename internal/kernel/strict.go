package kernel

import (
	"fmt"
	"github.com/cedar2025/xboard-node/internal/config"
	"github.com/cedar2025/xboard-node/internal/model"
	"net/netip"
)

func ValidateStrict(cfg config.KernelConfig, nc *model.NodeSpec) error {
	if nc.StrictDeviceLeases && !cfg.StrictDeviceLeases {
		return fmt.Errorf("panel requires strict_device_leases; enable it locally before serving users")
	}
	if !cfg.StrictDeviceLeases {
		return nil
	}
	if cfg.LeaseManager == nil || len(cfg.TrustedProxies) == 0 {
		return fmt.Errorf("strict leases require a lease manager and trusted_proxies")
	}
	if cfg.CustomConfig != "" {
		return fmt.Errorf("strict leases disallow custom_config extra inbounds")
	}
	if nc.Network != "" && nc.Network != "tcp" {
		return fmt.Errorf("strict leases currently require raw TCP transport")
	}
	if nc.Multiplex != nil && nc.Multiplex.Enabled {
		return fmt.Errorf("strict leases disallow multiplex")
	}
	switch nc.Protocol {
	case "vless", "vmess", "trojan", "shadowsocks":
	default:
		return fmt.Errorf("protocol not supported by strict TCP leases")
	}
	for _, cidr := range cfg.TrustedProxies {
		if _, err := netip.ParsePrefix(cidr); err != nil {
			return fmt.Errorf("invalid trusted proxy CIDR: %w", err)
		}
	}
	return nil
}
