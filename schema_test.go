package codexsdk

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTypeToJSONSchema(t *testing.T) {
	tests := []struct {
		name     string
		goType   string
		expected map[string]any
	}{
		{
			name:     "string type",
			goType:   "string",
			expected: map[string]any{"type": "string"},
		},
		{
			name:     "int type",
			goType:   "int",
			expected: map[string]any{"type": "integer"},
		},
		{
			name:     "int64 type",
			goType:   "int64",
			expected: map[string]any{"type": "integer"},
		},
		{
			name:     "float64 type",
			goType:   "float64",
			expected: map[string]any{"type": "number"},
		},
		{
			name:     "bool type",
			goType:   "bool",
			expected: map[string]any{"type": "boolean"},
		},
		{
			name:     "boolean type alias",
			goType:   "boolean",
			expected: map[string]any{"type": "boolean"},
		},
		{
			name:     "any type",
			goType:   "any",
			expected: map[string]any{"type": "object"},
		},
		{
			name:   "string array",
			goType: "[]string",
			expected: map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
		},
		{
			name:   "int array",
			goType: "[]int",
			expected: map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "integer"},
			},
		},
		{
			name:   "float64 array",
			goType: "[]float64",
			expected: map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "number"},
			},
		},
		{
			name:     "unknown type defaults to string",
			goType:   "unknown",
			expected: map[string]any{"type": "string"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := typeToJSONSchema(tt.goType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConvertSchema(t *testing.T) {
	t.Run("nil input returns empty object schema", func(t *testing.T) {
		result := convertSchema(nil)
		assert.Equal(t, "object", result["type"])
		assert.Equal(t, map[string]any{}, result["properties"])
	})

	t.Run("already JSON Schema is returned as-is", func(t *testing.T) {
		input := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
		}
		result := convertSchema(input)
		assert.Equal(t, input, result)
	})

	t.Run("simple type map is converted to JSON Schema", func(t *testing.T) {
		input := map[string]any{
			"a": "float64",
			"b": "string",
		}
		result := convertSchema(input)

		assert.Equal(t, "object", result["type"])

		properties, ok := result["properties"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, map[string]any{"type": "number"}, properties["a"])
		assert.Equal(t, map[string]any{"type": "string"}, properties["b"])

		required, ok := result["required"].([]string)
		require.True(t, ok)
		assert.Len(t, required, 2)
		assert.Contains(t, required, "a")
		assert.Contains(t, required, "b")
	})

	t.Run("map value is passed through as schema", func(t *testing.T) {
		customSchema := map[string]any{
			"type":        "string",
			"description": "Custom field",
		}
		input := map[string]any{
			"custom": customSchema,
		}
		result := convertSchema(input)

		properties, ok := result["properties"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, customSchema, properties["custom"])
	})
}

func TestSchemaBuilder(t *testing.T) {
	t.Run("builds schema with required properties", func(t *testing.T) {
		schema := NewSchemaBuilder().
			Property("name", "string").
			Property("age", "int").
			Build()

		assert.Equal(t, "object", schema["type"])

		properties, ok := schema["properties"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, map[string]any{"type": "string"}, properties["name"])
		assert.Equal(t, map[string]any{"type": "integer"}, properties["age"])

		required, ok := schema["required"].([]string)
		require.True(t, ok)
		assert.Equal(t, []string{"name", "age"}, required)
	})

	t.Run("builds schema with descriptions", func(t *testing.T) {
		schema := NewSchemaBuilder().
			PropertyWithDescription("email", "string", "User email address").
			Build()

		properties, ok := schema["properties"].(map[string]any)
		require.True(t, ok)
		emailProp, ok := properties["email"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "string", emailProp["type"])
		assert.Equal(t, "User email address", emailProp["description"])
	})

	t.Run("builds schema with optional properties", func(t *testing.T) {
		schema := NewSchemaBuilder().
			Property("name", "string").
			OptionalProperty("nickname", "string").
			Build()

		required, ok := schema["required"].([]string)
		require.True(t, ok)
		assert.Equal(t, []string{"name"}, required)

		properties, ok := schema["properties"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, properties, "nickname")
	})

	t.Run("builds schema with optional properties with description", func(t *testing.T) {
		schema := NewSchemaBuilder().
			OptionalPropertyWithDescription("bio", "string", "User biography").
			Build()

		properties, ok := schema["properties"].(map[string]any)
		require.True(t, ok)
		bioProp, ok := properties["bio"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "string", bioProp["type"])
		assert.Equal(t, "User biography", bioProp["description"])

		_, hasRequired := schema["required"]
		assert.False(t, hasRequired)
	})
}

func TestValidate(t *testing.T) {
	t.Run("passes when all required fields present", func(t *testing.T) {
		schema := map[string]any{
			"type":     "object",
			"required": []string{"name", "age"},
		}
		input := map[string]any{
			"name": "John",
			"age":  30,
		}
		err := Validate(schema, input)
		assert.NoError(t, err)
	})

	t.Run("fails when required field missing", func(t *testing.T) {
		schema := map[string]any{
			"type":     "object",
			"required": []string{"name", "age"},
		}
		input := map[string]any{
			"name": "John",
		}
		err := Validate(schema, input)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "age")
	})

	t.Run("handles required as []any from JSON", func(t *testing.T) {
		schema := map[string]any{
			"type":     "object",
			"required": []any{"name"},
		}
		input := map[string]any{}
		err := Validate(schema, input)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "name")
	})

	t.Run("passes with no required fields", func(t *testing.T) {
		schema := map[string]any{
			"type": "object",
		}
		input := map[string]any{}
		err := Validate(schema, input)
		assert.NoError(t, err)
	})
}
