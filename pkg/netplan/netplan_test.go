package netplan

import "testing"

func TestParse_ValidConfig(t *testing.T) {
	data := []byte(`network:
  version: 2
  renderer: networkd
  ethernets:
    eth0:
      addresses:
        - 192.168.1.10/24
      nameservers:
        addresses:
          - 8.8.8.8
      routes:
        - to: default
          via: 192.168.1.1
`)

	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("expected valid config, got: %v", err)
	}
	if cfg.Network.Version != 2 {
		t.Fatalf("expected version 2, got %d", cfg.Network.Version)
	}
	if _, ok := cfg.Network.Ethernets["eth0"]; !ok {
		t.Fatalf("expected eth0 ethernet config to exist")
	}
}

func TestParse_InvalidYAML(t *testing.T) {
	data := []byte(`network:
  version: two
`)

	if _, err := Parse(data); err == nil {
		t.Fatalf("expected invalid config to fail")
	}
}

func TestParse_InvalidVersion(t *testing.T) {
	data := []byte(`network:
  version: 1
`)

	if _, err := Parse(data); err == nil {
		t.Fatalf("expected unsupported version to fail")
	}
}
