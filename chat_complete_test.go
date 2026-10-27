package gai_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"maragu.dev/is"

	"maragu.dev/gai"
)

func TestPart_MarshalText(t *testing.T) {
	t.Run("includes MIME type and size for data parts", func(t *testing.T) {
		part := gai.DataPart("image/jpeg", []byte("fake image"))
		text, err := part.MarshalText()
		is.NotError(t, err)
		is.Equal(t, "[data: image/jpeg, 10 bytes]", string(text))
	})
}

func TestPart_MarshalJSON(t *testing.T) {
	t.Run("round-trips a part carrying metadata", func(t *testing.T) {
		part := gai.ThoughtPart("thinking hard")
		part.Metadata = gai.PartMetadata{Source: "example.com/client", Data: []byte("opaque")}

		data, err := json.Marshal(part)
		is.NotError(t, err)

		var got gai.Part
		is.NotError(t, json.Unmarshal(data, &got))

		is.Equal(t, gai.PartTypeThought, got.Type)
		is.Equal(t, "thinking hard", got.Thought())
		is.Equal(t, "example.com/client", got.Metadata.Source)
		is.Equal(t, "opaque", string(got.Metadata.Data))
		is.True(t, reflect.DeepEqual(part, got), "part should round-trip unchanged")
	})

	t.Run("omits metadata when there is none", func(t *testing.T) {
		data, err := json.Marshal(gai.TextPart("hi"))
		is.NotError(t, err)
		is.Equal(t, `{"type":"text","text":"hi"}`, string(data))
	})

	t.Run("writes the metadata envelope as source and data", func(t *testing.T) {
		// The wire shape is persisted in message histories, so it is pinned here: a
		// renamed field would strand every history already stored.
		part := gai.ThoughtPart("thinking hard")
		part.Metadata = gai.PartMetadata{Source: "example.com/client", Data: []byte("opaque")}

		data, err := json.Marshal(part)
		is.NotError(t, err)
		is.Equal(t, `{"type":"thought","text":"thinking hard","metadata":{"source":"example.com/client","data":"b3BhcXVl"}}`, string(data))
	})

	t.Run("refuses to write a part it could not read back", func(t *testing.T) {
		_, err := json.Marshal([]gai.Part{gai.TextPart("hi"), {Type: gai.PartTypeThought}})
		is.True(t, err != nil, "expected an error")
		is.True(t, strings.Contains(err.Error(), "thought part has no text"), err.Error())
	})

	t.Run("refuses to write tool call arguments that are not valid JSON", func(t *testing.T) {
		// A stream cut short mid-tool-call leaves truncated arguments behind, and a
		// history carrying those cannot be replayed against any provider.
		_, err := json.Marshal(gai.ToolCallPart("call-1", "read_file", json.RawMessage(`{"path":`)))
		is.True(t, err != nil, "expected an error")
		is.True(t, strings.Contains(err.Error(), "tool_call part has tool call arguments that are not valid JSON"), err.Error())
	})

	t.Run("leaves the part alone when unmarshalling a JSON null", func(t *testing.T) {
		part := gai.TextPart("hi")
		is.NotError(t, json.Unmarshal([]byte("null"), &part))
		is.Equal(t, "hi", part.Text())
	})

	t.Run("round-trips a tool result error with an empty message", func(t *testing.T) {
		// An error is an error even with nothing to say; dropping it would turn a failed
		// tool result back into a successful one on replay.
		part := gai.NewUserToolResultMessage(gai.ToolResult{
			ID: "call-1", Name: "read_file", Content: "partial output", Err: errors.New(""),
		}).Parts[0]

		data, err := json.Marshal(part)
		is.NotError(t, err)

		var got gai.Part
		is.NotError(t, json.Unmarshal(data, &got))

		is.True(t, got.ToolResult().Err != nil, "the error should survive")
		is.Equal(t, "", got.ToolResult().Err.Error())
	})

	t.Run("round-trips a message history with every part type", func(t *testing.T) {
		signedToolCall := gai.ToolCallPart("call-1", "read_file", json.RawMessage(`{"path":"readme.txt"}`))
		signedToolCall.Metadata = gai.PartMetadata{Source: "example.com/client", Data: []byte("sig-123")}

		messages := []gai.Message{
			gai.NewUserTextMessage("What is in the readme.txt file?"),
			gai.NewUserDataMessage("image/jpeg", []byte("fake image")),
			{Role: gai.MessageRoleModel, Parts: []gai.Part{
				gai.ThoughtPart("I should read the file"),
				gai.TextPart("Let me look."),
				signedToolCall,
			}},
			gai.NewUserToolResultMessage(gai.ToolResult{ID: "call-1", Name: "read_file", Content: "Hi!\n"}),
		}

		data, err := json.Marshal(messages)
		is.NotError(t, err)

		var got []gai.Message
		is.NotError(t, json.Unmarshal(data, &got))

		is.True(t, reflect.DeepEqual(messages, got), "message history should round-trip unchanged")
	})

	t.Run("rejects a part missing the content its type requires", func(t *testing.T) {
		tests := []struct {
			data     string
			expected string
		}{
			{data: `{"type":"text"}`, expected: "text part has no text"},
			{data: `{"type":"thought"}`, expected: "thought part has no text"},
			{data: `{"type":"data","mimeType":"image/jpeg"}`, expected: "data part has no data or MIME type"},
			{data: `{"type":"tool_call"}`, expected: "tool_call part has no tool call"},
			{data: `{"type":"tool_result"}`, expected: "tool_result part has no tool result"},
			// An unknown type would otherwise reach a client that panics on it, and a
			// history from a future version is exactly where one would come from.
			{data: `{"type":"video","data":"aGk=","mimeType":"video/mp4"}`, expected: `unknown part type "video"`},
			{data: `{}`, expected: `unknown part type ""`},
		}

		for _, test := range tests {
			t.Run(test.expected, func(t *testing.T) {
				var part gai.Part
				err := json.Unmarshal([]byte(test.data), &part)
				is.True(t, err != nil, "expected an error")
				is.Equal(t, test.expected, err.Error())
			})
		}
	})

	t.Run("round-trips a tool result error as its message", func(t *testing.T) {
		part := gai.NewUserToolResultMessage(gai.ToolResult{
			ID: "call-1", Name: "read_file", Err: errors.New("file not found"),
		}).Parts[0]

		data, err := json.Marshal(part)
		is.NotError(t, err)

		var got gai.Part
		is.NotError(t, json.Unmarshal(data, &got))

		is.Equal(t, "file not found", got.ToolResult().Err.Error())
	})
}

func TestDataPart(t *testing.T) {
	t.Run("creates a data part", func(t *testing.T) {
		data := []byte("image data")
		part := gai.DataPart("image/jpeg", data)

		is.Equal(t, gai.PartTypeData, part.Type)
		is.Equal(t, "image/jpeg", part.MIMEType)
		is.EqualSlice(t, data, part.Data)
	})

	t.Run("panics with empty MIME type", func(t *testing.T) {
		defer func() {
			r := recover()
			is.Equal(t, "MIME type must not be empty", r)
		}()

		gai.DataPart("", []byte("data"))
	})

	t.Run("panics with nil data", func(t *testing.T) {
		defer func() {
			r := recover()
			is.Equal(t, "data must not be empty", r)
		}()

		gai.DataPart("image/jpeg", nil)
	})

	t.Run("panics with empty data", func(t *testing.T) {
		defer func() {
			r := recover()
			is.Equal(t, "data must not be empty", r)
		}()

		gai.DataPart("image/jpeg", []byte{})
	})
}

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
			private string //nolint:unused // testing unexported field behavior
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

func TestSchema_MarshalJSON(t *testing.T) {
	t.Run("MinItems and MaxItems marshal as JSON numbers, not strings", func(t *testing.T) {
		minItems := int64(4)
		maxItems := int64(4)
		schema := gai.Schema{
			Type:     gai.SchemaTypeArray,
			MinItems: &minItems,
			MaxItems: &maxItems,
		}

		data, err := json.Marshal(schema)
		is.NotError(t, err)
		is.True(t, strings.Contains(string(data), `"minItems":4`))
		is.True(t, strings.Contains(string(data), `"maxItems":4`))
	})
}

func TestToolChoiceValidate(t *testing.T) {
	tools := []gai.Tool{
		{Name: "get_weather"},
		{Name: "get_time"},
	}

	t.Run("zero value is valid as auto", func(t *testing.T) {
		var tc gai.ToolChoice
		is.NotError(t, tc.Validate(tools))
	})

	t.Run("auto mode without name is valid", func(t *testing.T) {
		tc := gai.ToolChoice{Mode: gai.ToolChoiceModeAuto}
		is.NotError(t, tc.Validate(tools))
	})

	t.Run("any mode without name is valid", func(t *testing.T) {
		tc := gai.ToolChoice{Mode: gai.ToolChoiceModeAny}
		is.NotError(t, tc.Validate(tools))
	})

	t.Run("tool mode with matching name is valid", func(t *testing.T) {
		tc := gai.ToolChoice{Mode: gai.ToolChoiceModeTool, Name: "get_weather"}
		is.NotError(t, tc.Validate(tools))
	})

	t.Run("zero value with name is rejected", func(t *testing.T) {
		tc := gai.ToolChoice{Name: "get_weather"}
		err := tc.Validate(tools)
		is.True(t, err != nil)
		is.Equal(t, `tool choice name "get_weather" is only valid with mode "tool"`, err.Error())
	})

	t.Run("auto mode with name is rejected", func(t *testing.T) {
		tc := gai.ToolChoice{Mode: gai.ToolChoiceModeAuto, Name: "get_weather"}
		err := tc.Validate(tools)
		is.True(t, err != nil)
		is.Equal(t, `tool choice name "get_weather" is only valid with mode "tool"`, err.Error())
	})

	t.Run("any mode with name is rejected", func(t *testing.T) {
		tc := gai.ToolChoice{Mode: gai.ToolChoiceModeAny, Name: "get_weather"}
		err := tc.Validate(tools)
		is.True(t, err != nil)
		is.Equal(t, `tool choice name "get_weather" is only valid with mode "tool"`, err.Error())
	})

	t.Run("tool mode without name is rejected", func(t *testing.T) {
		tc := gai.ToolChoice{Mode: gai.ToolChoiceModeTool}
		err := tc.Validate(tools)
		is.True(t, err != nil)
		is.Equal(t, `tool choice mode "tool" requires a tool name`, err.Error())
	})

	t.Run("tool mode with unknown name is rejected", func(t *testing.T) {
		tc := gai.ToolChoice{Mode: gai.ToolChoiceModeTool, Name: "missing"}
		err := tc.Validate(tools)
		is.True(t, err != nil)
		is.Equal(t, `tool choice name "missing" does not match any provided tool`, err.Error())
	})

	t.Run("unknown mode is rejected", func(t *testing.T) {
		tc := gai.ToolChoice{Mode: gai.ToolChoiceMode("nonsense")}
		err := tc.Validate(tools)
		is.True(t, err != nil)
		is.Equal(t, `unknown tool choice mode "nonsense"`, err.Error())
	})
}
