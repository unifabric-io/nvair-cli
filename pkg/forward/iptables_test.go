package forward

import "testing"

func TestParseIPTablesTargets(t *testing.T) {
	output := `-A PREROUTING -p tcp --dport 20000 -m comment --comment "nvair cli port: 20000" -j DNAT --to-destination 192.168.200.6:6443
-A OUTPUT -p tcp --dport 20001 -m comment --comment "nvair cli port: 20001" -j DNAT --to-destination '[2001:db8::1]:443'
-A POSTROUTING -p tcp -d 192.168.200.6 --dport 6443 -m comment --comment "nvair cli port: 20000" -j MASQUERADE`

	targets := ParseIPTablesTargets(output)
	if len(targets) != 2 {
		t.Fatalf("expected 2 parsed DNAT targets, got %d", len(targets))
	}
	if targets[20000].Host != "192.168.200.6" || targets[20000].Port != 6443 {
		t.Fatalf("expected 20000 target 192.168.200.6:6443, got %#v", targets[20000])
	}
	if targets[20001].Host != "2001:db8::1" || targets[20001].Port != 443 {
		t.Fatalf("expected 20001 target [2001:db8::1]:443, got %#v", targets[20001])
	}
}

func TestParseCommentPorts(t *testing.T) {
	output := `-A PREROUTING -m comment --comment "nvair cli port: 20000"
-A OUTPUT -m comment --comment "nvair cli port: 20001"
-A OUTPUT -m comment --comment "nvair cli port: 70000"`

	ports := ParseCommentPorts(output)
	if len(ports) != 2 {
		t.Fatalf("expected 2 valid ports, got %d", len(ports))
	}
	if _, ok := ports[20000]; !ok {
		t.Fatalf("expected port 20000 to be parsed")
	}
	if _, ok := ports[20001]; !ok {
		t.Fatalf("expected port 20001 to be parsed")
	}
}

func TestPortComment(t *testing.T) {
	if got := PortComment(20000); got != "nvair cli port: 20000" {
		t.Fatalf("expected formatted port comment, got %q", got)
	}
}
