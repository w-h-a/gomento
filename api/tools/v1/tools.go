package v1

type ToolDefinition struct {
	Name        string
	Description string
	Schema      *Schema
}

type Schema struct {
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties,omitempty"`
	Required   []string       `json:"required,omitempty"`
}
