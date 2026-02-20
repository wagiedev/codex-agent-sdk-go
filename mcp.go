package codexsdk

import "context"

// Tool represents a custom tool that the agent can invoke.
//
// Tools allow users to extend the agent's capabilities with domain-specific
// functionality. When registered, the agent can discover and execute these
// tools during a session.
//
// Example:
//
//	tool := codexsdk.NewTool(
//	    "calculator",
//	    "Performs basic arithmetic operations",
//	    map[string]any{
//	        "type": "object",
//	        "properties": map[string]any{
//	            "operation": map[string]any{
//	                "type": "string",
//	                "enum": []string{"add", "subtract", "multiply", "divide"},
//	            },
//	            "a": map[string]any{"type": "number"},
//	            "b": map[string]any{"type": "number"},
//	        },
//	        "required": []string{"operation", "a", "b"},
//	    },
//	    func(ctx context.Context, input map[string]any) (map[string]any, error) {
//	        op := input["operation"].(string)
//	        a := input["a"].(float64)
//	        b := input["b"].(float64)
//
//	        var result float64
//	        switch op {
//	        case "add":
//	            result = a + b
//	        case "subtract":
//	            result = a - b
//	        case "multiply":
//	            result = a * b
//	        case "divide":
//	            if b == 0 {
//	                return nil, fmt.Errorf("division by zero")
//	            }
//	            result = a / b
//	        }
//
//	        return map[string]any{"result": result}, nil
//	    },
//	)
type Tool interface {
	// Name returns the unique identifier for this tool.
	Name() string

	// Description returns a human-readable description for the agent.
	Description() string

	// InputSchema returns a JSON schema describing expected input.
	// The schema should follow JSON Schema Draft 7 specification.
	InputSchema() map[string]any

	// Execute runs the tool with the provided input.
	// The input will be validated against InputSchema before execution.
	Execute(ctx context.Context, input map[string]any) (map[string]any, error)
}

// ToolFunc is a function-based tool implementation.
type ToolFunc func(ctx context.Context, input map[string]any) (map[string]any, error)

// NewTool creates a Tool from a function.
//
// This is a convenience constructor for creating tools without implementing
// the full Tool interface.
//
// Parameters:
//   - name: Unique identifier for the tool (e.g., "calculator", "search_database")
//   - description: Human-readable description of what the tool does
//   - schema: JSON Schema defining the expected input structure
//   - fn: Function that executes the tool logic
func NewTool(name, description string, schema map[string]any, fn ToolFunc) Tool {
	return &tool{
		name:        name,
		description: description,
		schema:      schema,
		fn:          fn,
	}
}

// tool is the internal tool implementation.
type tool struct {
	name        string
	description string
	schema      map[string]any
	fn          ToolFunc
}

// Compile-time verification that *tool implements the Tool interface.
var _ Tool = (*tool)(nil)

func (t *tool) Name() string                { return t.name }
func (t *tool) Description() string         { return t.description }
func (t *tool) InputSchema() map[string]any { return t.schema }
func (t *tool) Execute(ctx context.Context, input map[string]any) (map[string]any, error) {
	return t.fn(ctx, input)
}
