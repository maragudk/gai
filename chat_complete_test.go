package gai_test

import (
	"fmt"
	"testing"

	"maragu.dev/is"

	"maragu.dev/gai"
)

func TestGenerateSchema(t *testing.T) {
	t.Run("simple string type", func(t *testing.T) {
		type SimpleString struct {
			Name string `json:"name" jsonschema:"title=Name,description=The name field"`
		}

		schema := gai.GenerateSchema[SimpleString]()

		is.Equal(t, schema.Type, gai.SchemaTypeObject)
		is.Equal(t, len(schema.Properties), 1)

		nameSchema := schema.Properties["name"]
		is.NotNil(t, nameSchema)
		is.Equal(t, nameSchema.Type, gai.SchemaTypeString)
		is.Equal(t, nameSchema.Title, "Name")
		is.Equal(t, nameSchema.Description, "The name field")
	})

	t.Run("numeric types with constraints", func(t *testing.T) {
		type NumericTypes struct {
			Age    int     `json:"age" jsonschema:"minimum=0,maximum=150"`
			Height float64 `json:"height" jsonschema:"minimum=0.0,maximum=3.0"`
		}

		schema := gai.GenerateSchema[NumericTypes]()

		ageSchema := schema.Properties["age"]
		is.NotNil(t, ageSchema)
		is.Equal(t, ageSchema.Type, gai.SchemaTypeInteger)
		is.NotNil(t, ageSchema.Minimum)
		is.Equal(t, *ageSchema.Minimum, 0.0)
		is.NotNil(t, ageSchema.Maximum)
		is.Equal(t, *ageSchema.Maximum, 150.0)

		heightSchema := schema.Properties["height"]
		is.NotNil(t, heightSchema)
		is.Equal(t, heightSchema.Type, gai.SchemaTypeNumber)
		is.NotNil(t, heightSchema.Minimum)
		is.Equal(t, *heightSchema.Minimum, 0.0)
		is.NotNil(t, heightSchema.Maximum)
		is.Equal(t, *heightSchema.Maximum, 3.0)
	})

	t.Run("string with constraints", func(t *testing.T) {
		type StringConstraints struct {
			Username string `json:"username" jsonschema:"minLength=3,maxLength=20,pattern=^[a-zA-Z0-9_]+$"`
		}

		schema := gai.GenerateSchema[StringConstraints]()

		usernameSchema := schema.Properties["username"]
		is.NotNil(t, usernameSchema)
		is.Equal(t, usernameSchema.Type, gai.SchemaTypeString)
		is.NotNil(t, usernameSchema.MinLength)
		is.Equal(t, *usernameSchema.MinLength, int64(3))
		is.NotNil(t, usernameSchema.MaxLength)
		is.Equal(t, *usernameSchema.MaxLength, int64(20))
		is.Equal(t, usernameSchema.Pattern, "^[a-zA-Z0-9_]+$")
	})

	t.Run("enum field", func(t *testing.T) {
		type EnumField struct {
			Status string `json:"status" jsonschema:"enum=active,enum=inactive,enum=pending"`
		}

		schema := gai.GenerateSchema[EnumField]()

		statusSchema := schema.Properties["status"]
		is.NotNil(t, statusSchema)
		is.Equal(t, statusSchema.Type, gai.SchemaTypeString)
		is.Equal(t, len(statusSchema.Enum), 3)
		is.Equal(t, statusSchema.Enum[0], "active")
		is.Equal(t, statusSchema.Enum[1], "inactive")
		is.Equal(t, statusSchema.Enum[2], "pending")
	})

	t.Run("array types", func(t *testing.T) {
		type ArrayTypes struct {
			Tags   []string `json:"tags" jsonschema:"minItems=1,maxItems=10"`
			Scores []int    `json:"scores"`
		}

		schema := gai.GenerateSchema[ArrayTypes]()

		tagsSchema := schema.Properties["tags"]
		is.NotNil(t, tagsSchema)
		is.Equal(t, tagsSchema.Type, gai.SchemaTypeArray)
		is.NotNil(t, tagsSchema.MinItems)
		is.Equal(t, *tagsSchema.MinItems, int64(1))
		is.NotNil(t, tagsSchema.MaxItems)
		is.Equal(t, *tagsSchema.MaxItems, int64(10))
		is.NotNil(t, tagsSchema.Items)
		is.Equal(t, tagsSchema.Items.Type, gai.SchemaTypeString)

		scoresSchema := schema.Properties["scores"]
		is.NotNil(t, scoresSchema)
		is.Equal(t, scoresSchema.Type, gai.SchemaTypeArray)
		is.NotNil(t, scoresSchema.Items)
		is.Equal(t, scoresSchema.Items.Type, gai.SchemaTypeInteger)
	})

	t.Run("nested object", func(t *testing.T) {
		type Address struct {
			Street string `json:"street"`
			City   string `json:"city"`
		}
		type Person struct {
			Name    string  `json:"name"`
			Address Address `json:"address"`
		}

		schema := gai.GenerateSchema[Person]()

		is.Equal(t, schema.Type, gai.SchemaTypeObject)
		is.Equal(t, len(schema.Properties), 2)

		addressSchema := schema.Properties["address"]
		is.NotNil(t, addressSchema)
		is.Equal(t, addressSchema.Type, gai.SchemaTypeObject)
		is.Equal(t, len(addressSchema.Properties), 2)

		streetSchema := addressSchema.Properties["street"]
		is.NotNil(t, streetSchema)
		is.Equal(t, streetSchema.Type, gai.SchemaTypeString)

		citySchema := addressSchema.Properties["city"]
		is.NotNil(t, citySchema)
		is.Equal(t, citySchema.Type, gai.SchemaTypeString)
	})

	t.Run("mixed required and omitempty", func(t *testing.T) {
		type MixedRequirements struct {
			AlwaysRequired   string  `json:"always_required"`
			ExplicitRequired string  `json:"explicit_required" jsonschema:"required"`
			WithOmitempty    string  `json:"with_omitempty,omitempty"`
			PointerRequired  *string `json:"pointer_required"`
			PointerOmitempty *string `json:"pointer_omitempty,omitempty"`
		}

		schema := gai.GenerateSchema[MixedRequirements]()

		// Check which fields are required
		requiredMap := make(map[string]bool)
		for _, field := range schema.Required {
			requiredMap[field] = true
		}

		// Non-omitempty fields should be required
		is.True(t, requiredMap["always_required"])
		is.True(t, requiredMap["explicit_required"])
		is.True(t, requiredMap["pointer_required"]) // Even pointers without omitempty are required

		// Omitempty fields should NOT be required
		is.True(t, !requiredMap["with_omitempty"])
		is.True(t, !requiredMap["pointer_omitempty"])

		// Total required fields
		is.Equal(t, len(schema.Required), 3)
	})

	t.Run("property ordering", func(t *testing.T) {
		type OrderedProps struct {
			First  string `json:"first"`
			Second string `json:"second"`
			Third  string `json:"third"`
		}

		schema := gai.GenerateSchema[OrderedProps]()

		is.Equal(t, len(schema.PropertyOrdering), 3)
		is.Equal(t, schema.PropertyOrdering[0], "first")
		is.Equal(t, schema.PropertyOrdering[1], "second")
		is.Equal(t, schema.PropertyOrdering[2], "third")
	})

	t.Run("boolean type", func(t *testing.T) {
		type BooleanField struct {
			IsActive bool `json:"is_active"`
		}

		schema := gai.GenerateSchema[BooleanField]()

		isActiveSchema := schema.Properties["is_active"]
		is.NotNil(t, isActiveSchema)
		is.Equal(t, isActiveSchema.Type, gai.SchemaTypeBoolean)
	})

	t.Run("default and example values", func(t *testing.T) {
		type DefaultExample struct {
			Port int `json:"port" jsonschema:"default=8080,example=3000"`
		}

		schema := gai.GenerateSchema[DefaultExample]()

		portSchema := schema.Properties["port"]
		is.NotNil(t, portSchema)
		// Default and Example are stored as json.Number
		is.Equal(t, fmt.Sprint(portSchema.Default), "8080")
		is.Equal(t, fmt.Sprint(portSchema.Example), "3000")
	})

	t.Run("format field", func(t *testing.T) {
		type FormatField struct {
			Email string `json:"email" jsonschema:"format=email"`
			Date  string `json:"date" jsonschema:"format=date"`
		}

		schema := gai.GenerateSchema[FormatField]()

		emailSchema := schema.Properties["email"]
		is.NotNil(t, emailSchema)
		is.Equal(t, emailSchema.Format, "email")

		dateSchema := schema.Properties["date"]
		is.NotNil(t, dateSchema)
		is.Equal(t, dateSchema.Format, "date")
	})

	t.Run("pointer types", func(t *testing.T) {
		type PointerTypes struct {
			OptionalString *string `json:"optional_string"`
			OptionalInt    *int    `json:"optional_int"`
		}

		schema := gai.GenerateSchema[PointerTypes]()

		optStringSchema := schema.Properties["optional_string"]
		is.NotNil(t, optStringSchema)
		is.Equal(t, optStringSchema.Type, gai.SchemaTypeString)

		optIntSchema := schema.Properties["optional_int"]
		is.NotNil(t, optIntSchema)
		is.Equal(t, optIntSchema.Type, gai.SchemaTypeInteger)
	})

	t.Run("map type", func(t *testing.T) {
		type MapField struct {
			Metadata map[string]string `json:"metadata"`
		}

		schema := gai.GenerateSchema[MapField]()

		metadataSchema := schema.Properties["metadata"]
		is.NotNil(t, metadataSchema)
		is.Equal(t, metadataSchema.Type, gai.SchemaTypeObject)
	})

	t.Run("any type", func(t *testing.T) {
		type InterfaceField struct {
			Value any `json:"value"`
		}

		schema := gai.GenerateSchema[InterfaceField]()

		valueSchema := schema.Properties["value"]
		is.NotNil(t, valueSchema)
		// "any" is represented as an empty schema (allowing any type)
		// Just verify the field exists
	})

	t.Run("empty struct", func(t *testing.T) {
		type Empty struct{}

		schema := gai.GenerateSchema[Empty]()

		is.Equal(t, schema.Type, gai.SchemaTypeObject)
		is.Equal(t, len(schema.Properties), 0)
		is.Equal(t, len(schema.Required), 0)
	})

	t.Run("unexported fields ignored", func(t *testing.T) {
		type WithUnexported struct {
			Public  string `json:"public"`
			private string `json:"private"` //nolint:unused,govet // testing unexported field behavior
		}

		schema := gai.GenerateSchema[WithUnexported]()

		is.Equal(t, len(schema.Properties), 1)
		is.NotNil(t, schema.Properties["public"])
		_, exists := schema.Properties["private"]
		is.True(t, !exists)
	})

	t.Run("json tag with dash ignored", func(t *testing.T) {
		type WithIgnored struct {
			Name    string `json:"name"`
			Ignored string `json:"-"`
		}

		schema := gai.GenerateSchema[WithIgnored]()

		is.Equal(t, len(schema.Properties), 1)
		is.NotNil(t, schema.Properties["name"])
		_, exists := schema.Properties["-"]
		is.True(t, !exists)
	})

	t.Run("anonymous embedded struct", func(t *testing.T) {
		type Embedded struct {
			EmbeddedField string `json:"embedded_field"`
		}
		type WithEmbedded struct {
			Embedded
			OwnField string `json:"own_field"`
		}

		schema := gai.GenerateSchema[WithEmbedded]()

		// Both fields should be present at the top level
		is.Equal(t, len(schema.Properties), 2)
		is.NotNil(t, schema.Properties["embedded_field"])
		is.NotNil(t, schema.Properties["own_field"])
	})

	t.Run("struct with no json tags", func(t *testing.T) {
		type NoTags struct {
			FirstName string
			LastName  string
		}

		schema := gai.GenerateSchema[NoTags]()

		// Fields without json tags use their Go field names
		is.Equal(t, len(schema.Properties), 2)
		is.NotNil(t, schema.Properties["FirstName"])
		is.NotNil(t, schema.Properties["LastName"])
	})
}
