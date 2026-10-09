package create

import (
	"testing"

	"github.com/unifabric-io/nvair-cli/pkg/api"
)

func TestSwitchLinkLocalAddr(t *testing.T) {
	n := api.Node{
		Name: "switch-gpu-leaf3",
		ManagementInterfaces: map[string]api.ManagementInterface{
			"eth0": {IP: "192.168.200.113", MACAddress: "48:B0:2D:00:00:03"},
		},
	}

	if got, want := switchLinkLocalAddr(n, "eth1"), "[fe80::4ab0:2dff:fe00:3%eth1]:22"; got != want {
		t.Fatalf("switchLinkLocalAddr() = %q, want %q", got, want)
	}
	if got := switchLinkLocalAddr(n, ""); got != "" {
		t.Fatalf("expected empty address without interface, got %q", got)
	}
	if got := switchLinkLocalAddr(api.Node{Name: "no-mac"}, "eth1"); got != "" {
		t.Fatalf("expected empty address without MAC, got %q", got)
	}
}
