package topology

// RawTopology represents the actual topology.json file format from NVIDIA Air API
type RawTopology struct {
	Format  string             `json:"format"`
	Title   string             `json:"title"`
	ZTP     interface{}        `json:"ztp"`
	Content RawTopologyContent `json:"content"`
}

// RawTopologyContent represents the content section of topology.json
type RawTopologyContent struct {
	Nodes map[string]interface{} `json:"nodes"`
	Links []interface{}          `json:"links"`
	Other map[string]interface{} `json:"-"`
}

// ValidationError represents a single validation error
type ValidationError struct {
	Field   string
	Message string
	Path    string // file path if applicable
}

// ValidationResult contains all validation errors found
type ValidationResult struct {
	Valid  bool
	Errors []ValidationError
}

// DeleteRequest represents a resource deletion request
type DeleteRequest struct {
	ResourceType string // "simulation" or "service"
	ResourceName string
	ResourceID   string
}
