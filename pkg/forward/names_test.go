package forward

import "testing"

func TestBuildAndParseServiceName(t *testing.T) {
	name := BuildServiceName(20000, 6443, "node-gpu-1")
	if name != "forward-20000->node-gpu-1:6443" {
		t.Fatalf("unexpected service name: %q", name)
	}

	parsed, ok := ParseServiceName(name)
	if !ok {
		t.Fatalf("expected managed service name to parse")
	}
	if parsed.ServiceType != "" {
		t.Fatalf("unexpected service type: %q", parsed.ServiceType)
	}
	if parsed.ListenPort != 20000 {
		t.Fatalf("unexpected listen port: %d", parsed.ListenPort)
	}
	if parsed.TargetPort != 6443 {
		t.Fatalf("unexpected target port: %d", parsed.TargetPort)
	}
	if parsed.TargetHost != "node-gpu-1" {
		t.Fatalf("unexpected target host: %q", parsed.TargetHost)
	}
}

func TestParseServiceName_CompatibleWithLegacyFormat(t *testing.T) {
	parsed, ok := ParseServiceName("forward-ssh-10022->node-gpu-1:22")
	if !ok {
		t.Fatalf("expected legacy managed service name to parse")
	}
	if parsed.ServiceType != "ssh" {
		t.Fatalf("unexpected legacy service type: %q", parsed.ServiceType)
	}
	if parsed.ListenPort != 10022 {
		t.Fatalf("unexpected legacy listen port: %d", parsed.ListenPort)
	}
	if parsed.TargetPort != 22 {
		t.Fatalf("unexpected legacy target port: %d", parsed.TargetPort)
	}
	if parsed.TargetHost != "node-gpu-1" {
		t.Fatalf("unexpected legacy target host: %q", parsed.TargetHost)
	}
}

func TestRejectUnsupportedLegacyManagedServiceName(t *testing.T) {
	if _, ok := ParseServiceName("forward-https-6443_node-gpu-1"); ok {
		t.Fatalf("did not expect underscore legacy service name to parse")
	}
	if _, ok := ParseServiceName("forward-ssh-10022-node-gpu-1-22"); ok {
		t.Fatalf("did not expect hyphen legacy service name to parse")
	}
}

func TestIsBastionSSHServiceName(t *testing.T) {
	if !IsBastionSSHServiceName(BuildBastionSSHServiceName()) {
		t.Fatalf("expected managed bastion ssh service name to be recognized")
	}
	if !IsBastionSSHServiceName("forward-ssh-22->oob-mgmt-server:22") {
		t.Fatalf("expected legacy bastion ssh service name to be recognized")
	}
	if IsBastionSSHServiceName("forward-20000->node-gpu-1:22") {
		t.Fatalf("did not expect custom ssh forward to be treated as bastion ssh service")
	}
}
