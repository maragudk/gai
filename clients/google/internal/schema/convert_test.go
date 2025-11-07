package schema_test

import (
	"os"
	"testing"

	"google.golang.org/genai"
	"maragu.dev/gai"
	"maragu.dev/gai/tools"
	"maragu.dev/is"

	"maragu.dev/gai/clients/google/internal/schema"
)

func TestConvertToolToFunction(t *testing.T) {
	t.Run("converts ReadFile tool", func(t *testing.T) {
		root, err := os.OpenRoot("../../testdata")
		is.NotError(t, err)

		tool := tools.NewReadFile(root)
		funcDecl, err := schema.ConvertToolToFunction(tool)
		is.NotError(t, err)

		is.Equal(t, "read_file", funcDecl.Name)
		is.Equal(t, "Read the contents of a given relative file path. Use this when you want to see what's inside a file. Do not use this with directory names.", funcDecl.Description)

		// Check parameters
		is.NotError(t, err)
		is.Equal(t, genai.TypeObject, funcDecl.Parameters.Type)
		is.Equal(t, 1, len(funcDecl.Parameters.Properties))

		pathProp, ok := funcDecl.Parameters.Properties["path"]
		is.True(t, ok, "expected path property")
		is.Equal(t, genai.TypeString, pathProp.Type)
		is.Equal(t, "The relative path of a file in the working directory.", pathProp.Description)
	})

	t.Run("converts ListDir tool", func(t *testing.T) {
		root, err := os.OpenRoot("../../testdata")
		is.NotError(t, err)

		tool := tools.NewListDir(root)
		funcDecl, err := schema.ConvertToolToFunction(tool)
		is.NotError(t, err)

		is.Equal(t, "list_dir", funcDecl.Name)
		is.Equal(t, "List files and directories at a given path recursively. If no path is provided, lists files and directories in the current directory.", funcDecl.Description)

		// ListDir has a path parameter
		is.Equal(t, genai.TypeObject, funcDecl.Parameters.Type)
		is.Equal(t, 1, len(funcDecl.Parameters.Properties))

		pathProp, ok := funcDecl.Parameters.Properties["path"]
		is.True(t, ok, "expected path property")
		is.Equal(t, genai.TypeString, pathProp.Type)
	})
}

func TestConvertTools(t *testing.T) {
	t.Run("converts multiple tools", func(t *testing.T) {
		root, err := os.OpenRoot("../../testdata")
		is.NotError(t, err)

		gaiTools := []gai.Tool{
			tools.NewReadFile(root),
			tools.NewListDir(root),
		}

		genaiTools, err := schema.ConvertTools(gaiTools)
		is.NotError(t, err)

		is.Equal(t, 1, len(genaiTools))
		is.Equal(t, 2, len(genaiTools[0].FunctionDeclarations))

		is.Equal(t, "read_file", genaiTools[0].FunctionDeclarations[0].Name)
		is.Equal(t, "list_dir", genaiTools[0].FunctionDeclarations[1].Name)
	})
}

func TestConvertToolSchema(t *testing.T) {
	t.Run("converts empty schema", func(t *testing.T) {
		testSchema := gai.ToolSchema{}

		genaiSchema, err := schema.ConvertToolSchema(testSchema)
		is.NotError(t, err)

		is.Equal(t, genai.TypeObject, genaiSchema.Type)
		is.Equal(t, 0, len(genaiSchema.Properties))
	})

	t.Run("converts simple properties", func(t *testing.T) {
		toolSchema := gai.ToolSchema{
			Properties: map[string]*gai.Schema{
				"name": {
					Type:        gai.SchemaTypeString,
					Description: "The name",
				},
				"age": {
					Type:        gai.SchemaTypeInteger,
					Description: "The age",
				},
			},
		}

		genaiSchema, err := schema.ConvertToolSchema(toolSchema)
		is.NotError(t, err)

		is.Equal(t, genai.TypeObject, genaiSchema.Type)
		is.Equal(t, 2, len(genaiSchema.Properties))

		nameProp := genaiSchema.Properties["name"]
		is.Equal(t, genai.TypeString, nameProp.Type)
		is.Equal(t, "The name", nameProp.Description)

		ageProp := genaiSchema.Properties["age"]
		is.Equal(t, genai.TypeInteger, ageProp.Type)
		is.Equal(t, "The age", ageProp.Description)
	})

	t.Run("converts JSON Schema format with properties wrapper", func(t *testing.T) {
		toolSchema := gai.ToolSchema{
			Properties: map[string]*gai.Schema{
				"file": {
					Type:        gai.SchemaTypeString,
					Description: "File path",
					Required:    []string{"file"},
				},
			},
		}

		genaiSchema, err := schema.ConvertToolSchema(toolSchema)
		is.NotError(t, err)

		is.Equal(t, genai.TypeObject, genaiSchema.Type)
		is.Equal(t, 1, len(genaiSchema.Properties))

		fileProp := genaiSchema.Properties["file"]
		is.Equal(t, genai.TypeString, fileProp.Type)
		is.Equal(t, "File path", fileProp.Description)
	})

	t.Run("converts array type", func(t *testing.T) {
		toolSchema := gai.ToolSchema{
			Properties: map[string]*gai.Schema{
				"tags": {
					Type:        gai.SchemaTypeArray,
					Description: "List of tags",
					Items: &gai.Schema{
						Type: gai.SchemaTypeString,
					},
				},
			},
		}

		genaiSchema, err := schema.ConvertToolSchema(toolSchema)
		is.NotError(t, err)

		tagsProp := genaiSchema.Properties["tags"]
		is.Equal(t, genai.TypeArray, tagsProp.Type)
		is.Equal(t, "List of tags", tagsProp.Description)
		is.Equal(t, genai.TypeString, tagsProp.Items.Type)
	})

	t.Run("converts nested object type", func(t *testing.T) {
		toolSchema := gai.ToolSchema{
			Properties: map[string]*gai.Schema{
				"person": {
					Type:        gai.SchemaTypeObject,
					Description: "Person details",
					Properties: map[string]*gai.Schema{
						"name": {
							Type: gai.SchemaTypeString,
						},
						"age": {
							Type: gai.SchemaTypeInteger,
						},
					},
				},
			},
		}

		genaiSchema, err := schema.ConvertToolSchema(toolSchema)
		is.NotError(t, err)

		personProp := genaiSchema.Properties["person"]
		is.Equal(t, genai.TypeObject, personProp.Type)
		is.Equal(t, "Person details", personProp.Description)
		is.Equal(t, 2, len(personProp.Properties))

		is.Equal(t, genai.TypeString, personProp.Properties["name"].Type)
		is.Equal(t, genai.TypeInteger, personProp.Properties["age"].Type)
	})

	t.Run("converts all basic types", func(t *testing.T) {
		toolSchema := gai.ToolSchema{
			Properties: map[string]*gai.Schema{
				"text":    {Type: gai.SchemaTypeString},
				"number":  {Type: gai.SchemaTypeNumber},
				"integer": {Type: gai.SchemaTypeInteger},
				"boolean": {Type: gai.SchemaTypeBoolean},
				"unknown": {}, // Should default to string
			},
		}

		genaiSchema, err := schema.ConvertToolSchema(toolSchema)
		is.NotError(t, err)

		is.Equal(t, genai.TypeString, genaiSchema.Properties["text"].Type)
		is.Equal(t, genai.TypeNumber, genaiSchema.Properties["number"].Type)
		is.Equal(t, genai.TypeInteger, genaiSchema.Properties["integer"].Type)
		is.Equal(t, genai.TypeBoolean, genaiSchema.Properties["boolean"].Type)
		is.Equal(t, genai.TypeString, genaiSchema.Properties["unknown"].Type)
	})
}

func TestConvertResponseSchema(t *testing.T) {
	t.Run("converts simple object schema", func(t *testing.T) {
		inputSchema := gai.Schema{
			Type: gai.SchemaTypeObject,
			Properties: map[string]*gai.Schema{
				"title": {
					Type:        gai.SchemaTypeString,
					Description: "Book title",
				},
				"author": {
					Type:        gai.SchemaTypeString,
					Description: "Book author",
				},
				"year": {
					Type:        gai.SchemaTypeInteger,
					Description: "Publication year",
				},
			},
			Required: []string{"title", "author", "year"},
		}

		genaiSchema, err := schema.ConvertResponseSchema(inputSchema)
		is.NotError(t, err)

		is.Equal(t, genai.TypeObject, genaiSchema.Type)
		is.Equal(t, 3, len(genaiSchema.Properties))
		is.EqualSlice(t, []string{"title", "author", "year"}, genaiSchema.Required)

		titleProp := genaiSchema.Properties["title"]
		is.Equal(t, genai.TypeString, titleProp.Type)
		is.Equal(t, "Book title", titleProp.Description)

		authorProp := genaiSchema.Properties["author"]
		is.Equal(t, genai.TypeString, authorProp.Type)
		is.Equal(t, "Book author", authorProp.Description)

		yearProp := genaiSchema.Properties["year"]
		is.Equal(t, genai.TypeInteger, yearProp.Type)
		is.Equal(t, "Publication year", yearProp.Description)
	})

	t.Run("converts array schema", func(t *testing.T) {
		inputSchema := gai.Schema{
			Type: gai.SchemaTypeArray,
			Items: &gai.Schema{
				Type: gai.SchemaTypeString,
			},
			Description: "List of tags",
		}

		genaiSchema, err := schema.ConvertResponseSchema(inputSchema)
		is.NotError(t, err)

		is.Equal(t, genai.TypeArray, genaiSchema.Type)
		is.Equal(t, "List of tags", genaiSchema.Description)
		is.Equal(t, genai.TypeString, genaiSchema.Items.Type)
	})

	t.Run("converts nested object schema", func(t *testing.T) {
		inputSchema := gai.Schema{
			Type: gai.SchemaTypeObject,
			Properties: map[string]*gai.Schema{
				"person": {
					Type: gai.SchemaTypeObject,
					Properties: map[string]*gai.Schema{
						"name": {
							Type: gai.SchemaTypeString,
						},
						"age": {
							Type: gai.SchemaTypeInteger,
						},
					},
					Required: []string{"name"},
				},
			},
		}

		genaiSchema, err := schema.ConvertResponseSchema(inputSchema)
		is.NotError(t, err)

		is.Equal(t, genai.TypeObject, genaiSchema.Type)

		personProp := genaiSchema.Properties["person"]
		is.Equal(t, genai.TypeObject, personProp.Type)
		is.Equal(t, 2, len(personProp.Properties))
		is.EqualSlice(t, []string{"name"}, personProp.Required)

		is.Equal(t, genai.TypeString, personProp.Properties["name"].Type)
		is.Equal(t, genai.TypeInteger, personProp.Properties["age"].Type)
	})

	t.Run("converts array of objects schema", func(t *testing.T) {
		inputSchema := gai.Schema{
			Type: gai.SchemaTypeArray,
			Items: &gai.Schema{
				Type: gai.SchemaTypeObject,
				Properties: map[string]*gai.Schema{
					"id": {
						Type: gai.SchemaTypeInteger,
					},
					"name": {
						Type: gai.SchemaTypeString,
					},
				},
			},
		}

		genaiSchema, err := schema.ConvertResponseSchema(inputSchema)
		is.NotError(t, err)

		is.Equal(t, genai.TypeArray, genaiSchema.Type)
		is.Equal(t, genai.TypeObject, genaiSchema.Items.Type)
		is.Equal(t, 2, len(genaiSchema.Items.Properties))
		is.Equal(t, genai.TypeInteger, genaiSchema.Items.Properties["id"].Type)
		is.Equal(t, genai.TypeString, genaiSchema.Items.Properties["name"].Type)
	})

	t.Run("converts all basic types", func(t *testing.T) {
		testCases := []struct {
			name     string
			input    gai.SchemaType
			expected genai.Type
		}{
			{"string", gai.SchemaTypeString, genai.TypeString},
			{"number", gai.SchemaTypeNumber, genai.TypeNumber},
			{"integer", gai.SchemaTypeInteger, genai.TypeInteger},
			{"boolean", gai.SchemaTypeBoolean, genai.TypeBoolean},
			{"array", gai.SchemaTypeArray, genai.TypeArray},
			{"object", gai.SchemaTypeObject, genai.TypeObject},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				inputSchema := gai.Schema{Type: tc.input}
				genaiSchema, err := schema.ConvertResponseSchema(inputSchema)
				is.NotError(t, err)
				is.Equal(t, tc.expected, genaiSchema.Type)
			})
		}
	})

	t.Run("defaults to string for unspecified type", func(t *testing.T) {
		inputSchema := gai.Schema{
			Description: "Some field",
		}

		genaiSchema, err := schema.ConvertResponseSchema(inputSchema)
		is.NotError(t, err)

		is.Equal(t, genai.TypeString, genaiSchema.Type)
		is.Equal(t, "Some field", genaiSchema.Description)
	})

	t.Run("handles empty schema", func(t *testing.T) {
		inputSchema := gai.Schema{}

		genaiSchema, err := schema.ConvertResponseSchema(inputSchema)
		is.NotError(t, err)

		is.Equal(t, genai.TypeString, genaiSchema.Type) // Defaults to string
	})

	t.Run("copies all fields", func(t *testing.T) {
		inputSchema := gai.Schema{
			Type:        gai.SchemaTypeString,
			Description: "Test description",
			Default:     "default value",
			Enum:        []string{"option1", "option2"},
			Example:     "example value",
			Format:      "email",
			Title:       "Test Title",
		}

		genaiSchema, err := schema.ConvertResponseSchema(inputSchema)
		is.NotError(t, err)

		is.Equal(t, genai.TypeString, genaiSchema.Type)
		is.Equal(t, "Test description", genaiSchema.Description)
		is.Equal(t, "default value", genaiSchema.Default)
		is.EqualSlice(t, []string{"option1", "option2"}, genaiSchema.Enum)
		is.Equal(t, "example value", genaiSchema.Example)
		is.Equal(t, "email", genaiSchema.Format)
		is.Equal(t, "Test Title", genaiSchema.Title)
	})

	t.Run("copies numeric constraints", func(t *testing.T) {
		inputSchema := gai.Schema{
			Type:    gai.SchemaTypeNumber,
			Maximum: gai.Ptr(float64(100.5)),
			Minimum: gai.Ptr(float64(10.5)),
		}

		genaiSchema, err := schema.ConvertResponseSchema(inputSchema)
		is.NotError(t, err)

		is.Equal(t, genai.TypeNumber, genaiSchema.Type)
		is.Equal(t, float64(100.5), *genaiSchema.Maximum)
		is.Equal(t, float64(10.5), *genaiSchema.Minimum)
	})

	t.Run("copies array constraints", func(t *testing.T) {
		inputSchema := gai.Schema{
			Type:     gai.SchemaTypeArray,
			MaxItems: gai.Ptr(int64(50)),
			MinItems: gai.Ptr(int64(5)),
			Items: &gai.Schema{
				Type: gai.SchemaTypeString,
			},
		}

		genaiSchema, err := schema.ConvertResponseSchema(inputSchema)
		is.NotError(t, err)

		is.Equal(t, genai.TypeArray, genaiSchema.Type)
		is.Equal(t, int64(50), *genaiSchema.MaxItems)
		is.Equal(t, int64(5), *genaiSchema.MinItems)
		is.Equal(t, genai.TypeString, genaiSchema.Items.Type)
	})

	t.Run("copies object constraints", func(t *testing.T) {
		inputSchema := gai.Schema{
			Type:             gai.SchemaTypeObject,
			PropertyOrdering: []string{"first", "second", "third"},
			Properties: map[string]*gai.Schema{
				"first": {Type: gai.SchemaTypeString},
			},
		}

		genaiSchema, err := schema.ConvertResponseSchema(inputSchema)
		is.NotError(t, err)

		is.Equal(t, genai.TypeObject, genaiSchema.Type)
		is.EqualSlice(t, []string{"first", "second", "third"}, genaiSchema.PropertyOrdering)
	})

	t.Run("converts anyOf schemas", func(t *testing.T) {
		inputSchema := gai.Schema{
			Type: gai.SchemaTypeString,
			AnyOf: []*gai.Schema{
				{
					Type:        gai.SchemaTypeString,
					Description: "String option",
				},
				{
					Type:        gai.SchemaTypeInteger,
					Description: "Integer option",
				},
			},
		}

		genaiSchema, err := schema.ConvertResponseSchema(inputSchema)
		is.NotError(t, err)

		is.Equal(t, genai.TypeString, genaiSchema.Type)
		is.Equal(t, 2, len(genaiSchema.AnyOf))
		is.Equal(t, genai.TypeString, genaiSchema.AnyOf[0].Type)
		is.Equal(t, "String option", genaiSchema.AnyOf[0].Description)
		is.Equal(t, genai.TypeInteger, genaiSchema.AnyOf[1].Type)
		is.Equal(t, "Integer option", genaiSchema.AnyOf[1].Description)
	})
}
