// Package tool defines the Tool interface and Registry used by the agent loop
// to discover and invoke executable capabilities exposed to the LLM.
package tool

import (
	"context"
	"fmt"
	"sort"
)

// Tool is an executable capability the LLM can invoke.
type Tool interface {
	// Name returns the tool's unique identifier.
	Name() string
	// Description returns a human-readable description for the LLM.
	Description() string
	// Parameters returns the tool's parameter definitions.
	Parameters() []Parameter
	// Execute runs the tool with the given arguments.
	Execute(ctx context.Context, args map[string]string) (*Result, error)
}

// Parameter describes a single tool parameter.
type Parameter struct {
	Name        string
	Type        string // "string", "number", "boolean"
	Description string
	Required    bool
}

// Result holds the output of a tool execution.
type Result struct {
	Text string // text result fed back to the LLM
	Data []byte // binary data (e.g., image bytes) — routed by agent loop, not sent to LLM
}

// ToolCall represents a parsed tool invocation from an LLM response.
type ToolCall struct {
	Name string
	Args map[string]string
}

// Registry holds registered tools and provides lookup.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a tool to the registry. Returns an error if a tool with the
// same name is already registered.
func (r *Registry) Register(t Tool) error {
	if _, exists := r.tools[t.Name()]; exists {
		return fmt.Errorf("tool %q already registered", t.Name())
	}
	r.tools[t.Name()] = t
	return nil
}

// Get returns the tool with the given name, or false if not found.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// All returns all registered tools in alphabetical order.
func (r *Registry) All() []Tool {
	tools := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		tools = append(tools, t)
	}
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name() < tools[j].Name()
	})
	return tools
}
