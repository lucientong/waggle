package output

import (
	"reflect"
	"testing"
)

type schemaTestStruct struct {
	Name    string   `json:"name" jsonschema:"description=The person's name"`
	Age     int      `json:"age"`
	Tags    []string `json:"tags,omitempty"`
	private string   //nolint:unused
}

func TestSchemaFor_Struct(t *testing.T) {
	schema := SchemaFor[schemaTestStruct]()

	if schema["type"] != "object" {
		t.Fatalf("expected type=object, got %v", schema["type"])
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties map")
	}

	// Check name field.
	nameProp, ok := props["name"].(map[string]any)
	if !ok {
		t.Fatal("expected name property")
	}
	if nameProp["type"] != "string" {
		t.Errorf("name type: %v", nameProp["type"])
	}
	if nameProp["description"] != "The person's name" {
		t.Errorf("name description: %v", nameProp["description"])
	}

	// Check age field.
	ageProp, ok := props["age"].(map[string]any)
	if !ok {
		t.Fatal("expected age property")
	}
	if ageProp["type"] != "integer" {
		t.Errorf("age type: %v", ageProp["type"])
	}

	// Check tags field (array).
	tagsProp, ok := props["tags"].(map[string]any)
	if !ok {
		t.Fatal("expected tags property")
	}
	if tagsProp["type"] != "array" {
		t.Errorf("tags type: %v", tagsProp["type"])
	}

	// Check required: name and age should be required, tags should not (omitempty).
	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("expected required array")
	}
	if !containsStr(required, "name") || !containsStr(required, "age") {
		t.Errorf("required should contain name and age: %v", required)
	}
	if containsStr(required, "tags") {
		t.Error("tags should not be required (omitempty)")
	}

	// Private field should not appear.
	if _, ok := props["private"]; ok {
		t.Error("private field should not appear in schema")
	}
}

func TestSchemaFor_BasicTypes(t *testing.T) {
	tests := []struct {
		name     string
		schema   map[string]any
		expected string
	}{
		{"string", SchemaFor[string](), "string"},
		{"int", SchemaFor[int](), "integer"},
		{"float64", SchemaFor[float64](), "number"},
		{"bool", SchemaFor[bool](), "boolean"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.schema["type"] != tt.expected {
				t.Errorf("expected %s, got %v", tt.expected, tt.schema["type"])
			}
		})
	}
}

func TestSchemaFor_Slice(t *testing.T) {
	schema := SchemaFor[[]int]()
	if schema["type"] != "array" {
		t.Fatalf("expected array, got %v", schema["type"])
	}
	items, ok := schema["items"].(map[string]any)
	if !ok {
		t.Fatal("expected items map")
	}
	if items["type"] != "integer" {
		t.Errorf("items type: %v", items["type"])
	}
}

func TestSchemaFor_Map(t *testing.T) {
	schema := SchemaFor[map[string]int]()
	if schema["type"] != "object" {
		t.Fatalf("expected object, got %v", schema["type"])
	}
	addl, ok := schema["additionalProperties"].(map[string]any)
	if !ok {
		t.Fatal("expected additionalProperties")
	}
	if addl["type"] != "integer" {
		t.Errorf("additionalProperties type: %v", addl["type"])
	}
}

type enumTestStruct struct {
	Status string `json:"status" jsonschema:"description=Current status,enum=active|inactive|pending"`
}

func TestSchemaFor_Enum(t *testing.T) {
	schema := SchemaFor[enumTestStruct]()
	props := schema["properties"].(map[string]any)
	statusProp := props["status"].(map[string]any)

	enumVals, ok := statusProp["enum"].([]any)
	if !ok {
		t.Fatal("expected enum array")
	}
	expected := []string{"active", "inactive", "pending"}
	if len(enumVals) != len(expected) {
		t.Fatalf("expected %d enum values, got %d", len(expected), len(enumVals))
	}
	for i, v := range enumVals {
		if v != expected[i] {
			t.Errorf("enum[%d]: expected %q, got %q", i, expected[i], v)
		}
	}
}

func TestSchemaFor_Pointer(t *testing.T) {
	type WithPtr struct {
		Value *int `json:"value"`
	}
	schema := SchemaFor[WithPtr]()
	props := schema["properties"].(map[string]any)
	valProp := props["value"].(map[string]any)
	if valProp["type"] != "integer" {
		t.Errorf("pointer to int should yield integer, got %v", valProp["type"])
	}
}

func TestSchemaFor_NilType(t *testing.T) {
	// interface{} has nil reflect.Type
	schema := SchemaFor[any]()
	if schema["type"] != "object" {
		t.Errorf("expected object for nil type, got %v", schema["type"])
	}
}

func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func TestSchemaFor_Deterministic(t *testing.T) {
	// Schema generation should be deterministic.
	s1 := SchemaFor[schemaTestStruct]()
	s2 := SchemaFor[schemaTestStruct]()
	if !reflect.DeepEqual(s1, s2) {
		t.Error("schema generation is not deterministic")
	}
}
