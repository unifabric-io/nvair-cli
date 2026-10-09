package node

import (
	"testing"

	"github.com/unifabric-io/nvair-cli/pkg/api"
)

func TestParseNodeMetadata(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    *NodeMetadata
		wantErr bool
	}{
		{
			name:  "valid metadata",
			input: `{"mgmt_ip":"192.168.1.1"}`,
			want: &NodeMetadata{
				MgmtIP: "192.168.1.1",
			},
			wantErr: false,
		},
		{
			name:  "valid metadata with extra fields",
			input: `{"mgmt_ip":"10.0.0.5","extra":"field"}`,
			want: &NodeMetadata{
				MgmtIP: "10.0.0.5",
			},
			wantErr: false,
		},
		{
			name:    "empty string",
			input:   "",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid json",
			input:   `{"mgmt_ip":"192.168.1.1"`,
			want:    nil,
			wantErr: true,
		},
		{
			name:  "missing mgmt_ip",
			input: `{"other_field":"value"}`,
			want: &NodeMetadata{
				MgmtIP: "",
			},
			wantErr: false,
		},
		{
			name:  "null mgmt_ip",
			input: `{"mgmt_ip":null}`,
			want: &NodeMetadata{
				MgmtIP: "",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseNodeMetadata(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseNodeMetadata() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got.MgmtIP != tt.want.MgmtIP {
				t.Errorf("ParseNodeMetadata() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveMgmtIP(t *testing.T) {
	tests := []struct {
		name string
		node api.Node
		want string
	}{
		{
			name: "prefer top level management_ip",
			node: api.Node{
				ManagementIP: "192.168.200.10",
				ManagementInterfaces: map[string]api.ManagementInterface{
					"eth0": {IP: "192.168.200.114"},
				},
			},
			want: "192.168.200.10",
		},
		{
			name: "missing management ip and no management_interfaces",
			node: api.Node{},
			want: "",
		},
		{
			name: "fallback to management_interfaces when management_ip is absent",
			node: api.Node{
				ManagementInterfaces: map[string]api.ManagementInterface{
					"eth0": {IP: "192.168.200.114", MACAddress: "48:B0:2D:00:00:14"},
				},
			},
			want: "192.168.200.114",
		},
		{
			name: "management_interfaces picks lowest interface name deterministically",
			node: api.Node{
				ManagementInterfaces: map[string]api.ManagementInterface{
					"eth1": {IP: "192.168.200.1"},
					"eth0": {IP: "192.168.200.2"},
				},
			},
			want: "192.168.200.2",
		},
		{
			name: "management_interfaces with null/empty ip is treated as missing",
			node: api.Node{
				ManagementInterfaces: map[string]api.ManagementInterface{
					"eth0": {IP: "", MACAddress: "48:B0:2D:00:04:88"},
				},
			},
			want: "",
		},
		{
			name: "management_ip takes precedence over management_interfaces",
			node: api.Node{
				ManagementIP: "192.168.200.10",
				ManagementInterfaces: map[string]api.ManagementInterface{
					"eth0": {IP: "192.168.200.114"},
				},
			},
			want: "192.168.200.10",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveMgmtIP(tt.node)
			if err != nil {
				t.Fatalf("ResolveMgmtIP() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ResolveMgmtIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveImageID(t *testing.T) {
	tests := []struct {
		name string
		node api.Node
		want string
	}{
		{
			name: "prefer image",
			node: api.Node{
				Image: "img-new",
				OS:    "img-old",
			},
			want: "img-new",
		},
		{
			name: "fallback to os",
			node: api.Node{
				OS: "img-old",
			},
			want: "img-old",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveImageID(tt.node); got != tt.want {
				t.Fatalf("ResolveImageID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSortNodesByName(t *testing.T) {
	tests := []struct {
		name     string
		input    []api.Node
		expected []string
	}{
		{
			name: "single node",
			input: []api.Node{
				{Name: "node-1"},
			},
			expected: []string{"node-1"},
		},
		{
			name: "already sorted",
			input: []api.Node{
				{Name: "node-1"},
				{Name: "node-2"},
				{Name: "node-3"},
			},
			expected: []string{"node-1", "node-2", "node-3"},
		},
		{
			name: "reverse sorted",
			input: []api.Node{
				{Name: "node-3"},
				{Name: "node-2"},
				{Name: "node-1"},
			},
			expected: []string{"node-1", "node-2", "node-3"},
		},
		{
			name: "mixed order",
			input: []api.Node{
				{Name: "node-10"},
				{Name: "node-2"},
				{Name: "node-1"},
				{Name: "node-20"},
			},
			expected: []string{"node-1", "node-2", "node-10", "node-20"},
		},
		{
			name: "gpu nodes",
			input: []api.Node{
				{Name: "node-gpu-3"},
				{Name: "node-gpu-1"},
				{Name: "node-gpu-2"},
			},
			expected: []string{"node-gpu-1", "node-gpu-2", "node-gpu-3"},
		},
		{
			name: "mixed node types",
			input: []api.Node{
				{Name: "node-gpu-2"},
				{Name: "node-storage-1"},
				{Name: "node-gpu-1"},
				{Name: "node-storage-2"},
			},
			expected: []string{"node-storage-1", "node-gpu-1", "node-gpu-2", "node-storage-2"},
		},
		{
			name: "large numbers",
			input: []api.Node{
				{Name: "node-100"},
				{Name: "node-20"},
				{Name: "node-1000"},
				{Name: "node-5"},
			},
			expected: []string{"node-5", "node-20", "node-100", "node-1000"},
		},
		{
			name:     "empty list",
			input:    []api.Node{},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SortNodesByName(tt.input)

			if len(tt.input) != len(tt.expected) {
				t.Errorf("SortNodesByName() length: got %d, want %d", len(tt.input), len(tt.expected))
				return
			}

			for i, node := range tt.input {
				if node.Name != tt.expected[i] {
					t.Errorf("SortNodesByName() at index %d: got %s, want %s", i, node.Name, tt.expected[i])
				}
			}
		})
	}
}

func TestExtractNodeNumber(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "simple node",
			input:    "node-1",
			expected: 1,
		},
		{
			name:     "gpu node",
			input:    "node-gpu-5",
			expected: 5,
		},
		{
			name:     "storage node",
			input:    "node-storage-10",
			expected: 10,
		},
		{
			name:     "large number",
			input:    "node-9999",
			expected: 9999,
		},
		{
			name:     "zero number",
			input:    "node-0",
			expected: 0,
		},
		{
			name:     "no number",
			input:    "node",
			expected: 0,
		},
		{
			name:     "number at start",
			input:    "1-node",
			expected: 1,
		},
		{
			name:     "multiple numbers",
			input:    "node-1-gpu-2",
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractNodeNumber(tt.input)
			if got != tt.expected {
				t.Errorf("extractNodeNumber(%q) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseNumber(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "valid positive number",
			input:    "123",
			expected: 123,
		},
		{
			name:     "zero",
			input:    "0",
			expected: 0,
		},
		{
			name:     "single digit",
			input:    "5",
			expected: 5,
		},
		{
			name:     "invalid - empty string",
			input:    "",
			expected: -1,
		},
		{
			name:     "invalid - text",
			input:    "abc",
			expected: -1,
		},
		{
			name:     "mixed - parses leading digits",
			input:    "12abc",
			expected: 12,
		},
		{
			name:     "spaces - parses leading digits",
			input:    "12 34",
			expected: 12,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseNumber(tt.input)
			if got != tt.expected {
				t.Errorf("parseNumber(%q) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}

func BenchmarkSortNodesByName(b *testing.B) {
	nodes := []api.Node{
		{Name: "node-100"},
		{Name: "node-20"},
		{Name: "node-1000"},
		{Name: "node-5"},
		{Name: "node-gpu-50"},
		{Name: "node-storage-75"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Create a copy to avoid modifying the original
		nodesCopy := make([]api.Node, len(nodes))
		copy(nodesCopy, nodes)
		SortNodesByName(nodesCopy)
	}
}

func TestLinkLocalFromMAC(t *testing.T) {
	tests := []struct {
		mac     string
		want    string
		wantErr bool
	}{
		{mac: "48:B0:2D:00:00:03", want: "fe80::4ab0:2dff:fe00:3"},
		{mac: "48:b0:2d:00:04:7c", want: "fe80::4ab0:2dff:fe00:47c"},
		{mac: "4a:b0:2d:00:00:03", want: "fe80::48b0:2dff:fe00:3"},
		{mac: " 00:00:00:00:00:00 ", want: "fe80::200:ff:fe00:0"},
		{mac: "", wantErr: true},
		{mac: "not-a-mac", wantErr: true},
		{mac: "48:b0:2d:00:00:03:00:01", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.mac, func(t *testing.T) {
			got, err := LinkLocalFromMAC(tt.mac)
			if (err != nil) != tt.wantErr {
				t.Fatalf("LinkLocalFromMAC(%q) error = %v, wantErr %v", tt.mac, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("LinkLocalFromMAC(%q) = %q, want %q", tt.mac, got, tt.want)
			}
		})
	}
}

func TestResolveMgmtMAC(t *testing.T) {
	tests := []struct {
		name string
		node api.Node
		want string
	}{
		{name: "no interfaces", node: api.Node{}, want: ""},
		{
			name: "interface carrying the management IP wins",
			node: api.Node{
				ManagementIP: "10.0.0.2",
				ManagementInterfaces: map[string]api.ManagementInterface{
					"eth0": {IP: "10.0.0.1", MACAddress: "48:B0:2D:00:00:01"},
					"eth1": {IP: "10.0.0.2", MACAddress: "48:B0:2D:00:00:02"},
				},
			},
			want: "48:B0:2D:00:00:02",
		},
		{
			name: "falls back to first interface with a MAC",
			node: api.Node{
				ManagementInterfaces: map[string]api.ManagementInterface{
					"eth0": {IP: "10.0.0.1"},
					"eth1": {IP: "10.0.0.2", MACAddress: " 48:B0:2D:00:00:02 "},
				},
			},
			want: "48:B0:2D:00:00:02",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveMgmtMAC(tt.node); got != tt.want {
				t.Fatalf("ResolveMgmtMAC() = %q, want %q", got, tt.want)
			}
		})
	}
}
