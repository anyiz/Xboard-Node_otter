package machine

import (
	"github.com/cedar2025/xboard-node/internal/config"
	"github.com/cedar2025/xboard-node/internal/panel"
	"testing"
)

func TestPolicyPreservesLegacyAndAppliesManagedSettings(t *testing.T) {
	cfg := config.KernelConfig{StrictDeviceLeases: true, TrustedProxies: []string{"127.0.0.1/32"}}
	if err := applyDevicePolicy(&cfg, nil); err != nil || !cfg.StrictDeviceLeases {
		t.Fatal("legacy config changed", err)
	}
	p := &panel.DevicePolicy{Enabled: true, TrustedProxies: []string{"192.0.2.1/32", "2001:db8::1/128"}, Revision: "r1"}
	if err := applyDevicePolicy(&cfg, p); err != nil {
		t.Fatal(err)
	}
	if !cfg.StrictDeviceLeases || cfg.DevicePolicyRevision != "r1" || cfg.TrustedProxies[0] != "192.0.2.1/32" {
		t.Fatal(cfg)
	}
	p.TrustedProxies[0] = "changed"
	if cfg.TrustedProxies[0] == "changed" {
		t.Fatal("policy aliases config")
	}
	if err := applyDevicePolicy(&cfg, &panel.DevicePolicy{Revision: "r2"}); err != nil || cfg.StrictDeviceLeases {
		t.Fatal("disable failed", err)
	}
}
func TestInvalidPolicyDoesNotMutateLocalConfig(t *testing.T) {
	for _, p := range []panel.DevicePolicy{
		{Enabled: true, Revision: "r"},
		{Enabled: true, Revision: "r", TrustedProxies: []string{"0.0.0.0/0"}},
		{Enabled: true, Revision: "r", TrustedProxies: []string{"::/0"}},
		{Enabled: true, Revision: "r", TrustedProxies: []string{"not-an-ip"}},
		{Enabled: true, TrustedProxies: []string{"192.0.2.1/32"}},
	} {
		cfg := config.KernelConfig{DevicePolicyRevision: "old"}
		if err := applyDevicePolicy(&cfg, &p); err == nil {
			t.Fatal("accepted invalid policy", p)
		}
		if cfg.DevicePolicyRevision != "old" {
			t.Fatal("partial mutation")
		}
	}
}
func TestPolicyChangeTriggersDifferentDiscoveryKey(t *testing.T) {
	a := panel.MachineNode{ID: 1, DevicePolicy: &panel.DevicePolicy{Enabled: true, Revision: "r1", TrustedProxies: []string{"192.0.2.1/32"}}}
	b := a
	b.DevicePolicy = &panel.DevicePolicy{Enabled: true, Revision: "r2", TrustedProxies: []string{"192.0.2.2/32"}}
	if policyKey(a) == policyKey(b) {
		t.Fatal("change not detected")
	}
	done := make(chan struct{})
	if nodeExited(done) {
		t.Fatal("running considered exited")
	}
	close(done)
	if !nodeExited(done) {
		t.Fatal("exit not detected")
	}
}
