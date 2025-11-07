package schema

import (
	"fmt"

	"google.golang.org/genai"
	"maragu.dev/gai"
)

// ConvertTools converts gai.Tool slice to genai.Tool slice.
func ConvertTools(tools []gai.Tool) ([]*genai.Tool, error) {
	var funcDecls []*genai.FunctionDeclaration
	for _, tool := range tools {
		funcDecl, err := ConvertToolToFunction(tool)
		if err != nil {
			return nil, fmt.Errorf("converting tool %s: %w", tool.Name, err)
		}
		funcDecls = append(funcDecls, funcDecl)
	}
	return []*genai.Tool{{FunctionDeclarations: funcDecls}}, nil
}

// ConvertToolToFunction converts a gai.Tool to genai.FunctionDeclaration.
func ConvertToolToFunction(tool gai.Tool) (*genai.FunctionDeclaration, error) {
	schema, err := ConvertToolSchema(tool.Schema)
	if err != nil {
		return nil, fmt.Errorf("converting schema: %w", err)
	}

	return &genai.FunctionDeclaration{
		Name:        tool.Name,
		Description: tool.Description,
		Parameters:  schema,
	}, nil
}

// ConvertToolSchema converts gai.ToolSchema to genai.Schema.
func ConvertToolSchema(schema gai.ToolSchema) (*genai.Schema, error) {
	genaiProps := make(map[string]*genai.Schema, len(schema.Properties))

	// Convert each property from gai.Schema to genai.Schema
	for name, prop := range schema.Properties {
		propSchema, err := ConvertResponseSchema(*prop)
		if err != nil {
			return nil, fmt.Errorf("converting property %s: %w", name, err)
		}
		genaiProps[name] = propSchema
	}

	return &genai.Schema{
		Type:       genai.TypeObject,
		Properties: genaiProps,
	}, nil
}

// ConvertResponseSchema converts gai.Schema to genai.Schema.
func ConvertResponseSchema(schema gai.Schema) (*genai.Schema, error) {
	result := &genai.Schema{}

	// Convert type
	switch schema.Type {
	case gai.SchemaTypeString:
		result.Type = genai.TypeString
	case gai.SchemaTypeNumber:
		result.Type = genai.TypeNumber
	case gai.SchemaTypeInteger:
		result.Type = genai.TypeInteger
	case gai.SchemaTypeBoolean:
		result.Type = genai.TypeBoolean
	case gai.SchemaTypeArray:
		result.Type = genai.TypeArray
		if schema.Items != nil {
			itemSchema, err := ConvertResponseSchema(*schema.Items)
			if err != nil {
				return nil, fmt.Errorf("converting array items: %w", err)
			}
			result.Items = itemSchema
		}
	case gai.SchemaTypeObject:
		result.Type = genai.TypeObject
		if len(schema.Properties) > 0 {
			result.Properties = make(map[string]*genai.Schema, len(schema.Properties))
			for name, prop := range schema.Properties {
				propSchema, err := ConvertResponseSchema(*prop)
				if err != nil {
					return nil, fmt.Errorf("converting property %s: %w", name, err)
				}
				result.Properties[name] = propSchema
			}
		}
		result.Required = schema.Required
	default:
		// Default to string if type is not specified
		result.Type = genai.TypeString
	}

	// Copy all other fields
	result.Description = schema.Description
	result.Default = schema.Default
	result.Enum = schema.Enum
	result.Example = schema.Example
	result.Format = schema.Format
	result.MaxItems = schema.MaxItems
	result.Maximum = schema.Maximum
	result.MinItems = schema.MinItems
	result.Minimum = schema.Minimum
	result.PropertyOrdering = schema.PropertyOrdering
	result.Title = schema.Title

	// Handle AnyOf recursively
	if schema.AnyOf != nil {
		result.AnyOf = make([]*genai.Schema, len(schema.AnyOf))
		for i, anyOfSchema := range schema.AnyOf {
			convertedSchema, err := ConvertResponseSchema(*anyOfSchema)
			if err != nil {
				return nil, fmt.Errorf("converting anyOf[%d]: %w", i, err)
			}
			result.AnyOf[i] = convertedSchema
		}
	}

	return result, nil
}
