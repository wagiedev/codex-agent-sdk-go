package codexsdk

import (
	"fmt"
	"strings"
)

// convertSchema converts a simple type map to a full JSON Schema.
//
// Input:  map[string]any{"a": "float64", "b": "string"}
// Output: map[string]any{
//
//	"type": "object",
//	"properties": map[string]any{
//	    "a": map[string]any{"type": "number"},
//	    "b": map[string]any{"type": "string"},
//	},
//	"required": []string{"a", "b"},
//
// }
//
// If the input already looks like a JSON Schema (has "type" key), it's returned as-is.
func convertSchema(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}

	// If already a JSON Schema (has "type" key at top level), return as-is
	if _, hasType := input["type"]; hasType {
		return input
	}

	properties := make(map[string]any, len(input))
	required := make([]string, 0, len(input))

	for name, typeVal := range input {
		typeStr, ok := typeVal.(string)
		if !ok {
			// If not a string, assume it's already a schema definition
			if schemaMap, isMap := typeVal.(map[string]any); isMap {
				properties[name] = schemaMap
			} else {
				// Fallback to string type
				properties[name] = map[string]any{"type": "string"}
			}
		} else {
			properties[name] = typeToJSONSchema(typeStr)
		}

		required = append(required, name)
	}

	return map[string]any{
		"type":       "object",
		"properties": properties,
		"required":   required,
	}
}

// typeToJSONSchema converts a Go type string to a JSON Schema type definition.
//
// Type mappings:
//   - "string"           → {"type": "string"}
//   - "int", "int64"     → {"type": "integer"}
//   - "float64", "float" → {"type": "number"}
//   - "bool"             → {"type": "boolean"}
//   - "[]string"         → {"type": "array", "items": {"type": "string"}}
//   - "[]int"            → {"type": "array", "items": {"type": "integer"}}
//   - "[]float64"        → {"type": "array", "items": {"type": "number"}}
//   - "any", "object"    → {"type": "object"}
func typeToJSONSchema(goType string) map[string]any {
	if itemType, found := strings.CutPrefix(goType, "[]"); found {
		return map[string]any{
			"type":  "array",
			"items": typeToJSONSchema(itemType),
		}
	}

	switch goType {
	case "string":
		return map[string]any{"type": "string"}
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64":
		return map[string]any{"type": "integer"}
	case "float32", "float64", "float", "number":
		return map[string]any{"type": "number"}
	case "bool", "boolean":
		return map[string]any{"type": "boolean"}
	case "any", "object", "map[string]any":
		return map[string]any{"type": "object"}
	default:
		return map[string]any{"type": "string"}
	}
}

// SchemaBuilder provides a fluent interface for building JSON schemas.
type SchemaBuilder struct {
	properties map[string]any
	required   []string
}

// NewSchemaBuilder creates a new SchemaBuilder.
func NewSchemaBuilder() *SchemaBuilder {
	return &SchemaBuilder{
		properties: make(map[string]any, 4),
		required:   make([]string, 0, 4),
	}
}

// Property adds a required property with the given name and Go type.
func (b *SchemaBuilder) Property(name, goType string) *SchemaBuilder {
	b.properties[name] = typeToJSONSchema(goType)
	b.required = append(b.required, name)

	return b
}

// PropertyWithDescription adds a required property with type and description.
func (b *SchemaBuilder) PropertyWithDescription(name, goType, description string) *SchemaBuilder {
	schema := typeToJSONSchema(goType)
	schema["description"] = description
	b.properties[name] = schema
	b.required = append(b.required, name)

	return b
}

// OptionalProperty adds an optional property (not in required list).
func (b *SchemaBuilder) OptionalProperty(name, goType string) *SchemaBuilder {
	b.properties[name] = typeToJSONSchema(goType)

	return b
}

// OptionalPropertyWithDescription adds an optional property with description.
func (b *SchemaBuilder) OptionalPropertyWithDescription(
	name, goType, description string,
) *SchemaBuilder {
	schema := typeToJSONSchema(goType)
	schema["description"] = description
	b.properties[name] = schema

	return b
}

// Build returns the complete JSON Schema.
func (b *SchemaBuilder) Build() map[string]any {
	schema := map[string]any{
		"type":       "object",
		"properties": b.properties,
	}

	if len(b.required) > 0 {
		schema["required"] = b.required
	}

	return schema
}

// Validate checks if the input matches the schema requirements.
// Returns an error if required fields are missing.
func Validate(schema, input map[string]any) error {
	required, _ := schema["required"].([]string)
	if required == nil {
		if reqAny, ok := schema["required"].([]any); ok {
			required = make([]string, 0, len(reqAny))
			for _, r := range reqAny {
				if s, isStr := r.(string); isStr {
					required = append(required, s)
				}
			}
		}
	}

	for _, field := range required {
		if _, exists := input[field]; !exists {
			return fmt.Errorf("missing required field: %s", field)
		}
	}

	return nil
}
