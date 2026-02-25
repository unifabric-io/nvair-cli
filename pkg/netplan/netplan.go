package netplan

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents a Netplan v2 configuration file.
type Config struct {
	Network Network `yaml:"network"`
}

// Network represents the top-level network section.
type Network struct {
	Version   int               `yaml:"version"`
	Renderer  string            `yaml:"renderer,omitempty"`
	Ethernets map[string]Device `yaml:"ethernets,omitempty"`
	Bonds     map[string]Device `yaml:"bonds,omitempty"`
	Bridges   map[string]Device `yaml:"bridges,omitempty"`
	Vlans     map[string]Device `yaml:"vlans,omitempty"`
}

// Device represents a Netplan network device definition.
type Device struct {
	Addresses     []string         `yaml:"addresses,omitempty"`
	Nameservers   *Nameservers     `yaml:"nameservers,omitempty"`
	Routes        []Route          `yaml:"routes,omitempty"`
	RoutingPolicy []RoutingPolicy  `yaml:"routing-policy,omitempty"`
	DHCP4         bool             `yaml:"dhcp4,omitempty"`
	DHCP6         bool             `yaml:"dhcp6,omitempty"`
	Gateway4      string           `yaml:"gateway4,omitempty"`
	Gateway6      string           `yaml:"gateway6,omitempty"`
	Renderer      string           `yaml:"renderer,omitempty"`
	Interfaces    []string         `yaml:"interfaces,omitempty"`
	Parameters    *DeviceParameter `yaml:"parameters,omitempty"`
	MTU           int              `yaml:"mtu,omitempty"`
	Optional      bool             `yaml:"optional,omitempty"`
	Link          string           `yaml:"link,omitempty"`
	ID            int              `yaml:"id,omitempty"`
	SetName       string           `yaml:"set-name,omitempty"`
	Match         *Match           `yaml:"match,omitempty"`
}

// Nameservers represents a Netplan nameserver definition.
type Nameservers struct {
	Addresses []string `yaml:"addresses,omitempty"`
	Search    []string `yaml:"search,omitempty"`
}

// Route represents a Netplan route definition.
type Route struct {
	To      string `yaml:"to,omitempty"`
	Via     string `yaml:"via,omitempty"`
	From    string `yaml:"from,omitempty"`
	Table   int    `yaml:"table,omitempty"`
	Metric  int    `yaml:"metric,omitempty"`
	Scope   string `yaml:"scope,omitempty"`
	Type    string `yaml:"type,omitempty"`
	OnLink  bool   `yaml:"on-link,omitempty"`
	MTU     int    `yaml:"mtu,omitempty"`
	FromAll bool   `yaml:"from-all,omitempty"`
}

// RoutingPolicy represents a Netplan routing-policy entry.
type RoutingPolicy struct {
	From     string `yaml:"from,omitempty"`
	To       string `yaml:"to,omitempty"`
	Table    int    `yaml:"table,omitempty"`
	Priority int    `yaml:"priority,omitempty"`
	Mark     int    `yaml:"mark,omitempty"`
	TypeOfS  int    `yaml:"type-of-service,omitempty"`
}

// DeviceParameter represents common bridge and bond parameters.
type DeviceParameter struct {
	Mode            string `yaml:"mode,omitempty"`
	Primary         string `yaml:"primary,omitempty"`
	TransmitHash    string `yaml:"transmit-hash-policy,omitempty"`
	LACP            string `yaml:"lacp-rate,omitempty"`
	STP             bool   `yaml:"stp,omitempty"`
	ForwardDelay    int    `yaml:"forward-delay,omitempty"`
	HelloTime       int    `yaml:"hello-time,omitempty"`
	MaxAge          int    `yaml:"max-age,omitempty"`
	MinLinks        int    `yaml:"min-links,omitempty"`
	MII             int    `yaml:"mii-monitor-interval,omitempty"`
	UpDelay         int    `yaml:"up-delay,omitempty"`
	DownDelay       int    `yaml:"down-delay,omitempty"`
	GratuitousARP   int    `yaml:"gratuitous-arp,omitempty"`
	AllSlavesActive bool   `yaml:"all-slaves-active,omitempty"`
}

// Match represents interface matching fields.
type Match struct {
	Name       string `yaml:"name,omitempty"`
	MACAddress string `yaml:"macaddress,omitempty"`
	Driver     string `yaml:"driver,omitempty"`
}

// Parse unmarshals a Netplan YAML document and performs basic Netplan v2 checks.
func Parse(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse netplan yaml: %w", err)
	}

	if cfg.Network.Version == 0 {
		return nil, fmt.Errorf("missing network.version")
	}
	if cfg.Network.Version != 2 {
		return nil, fmt.Errorf("unsupported netplan version %d", cfg.Network.Version)
	}

	return &cfg, nil
}

// LoadFile reads and parses a Netplan YAML file.
func LoadFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read netplan file: %w", err)
	}

	return Parse(data)
}
