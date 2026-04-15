package output

import (
	"reflect"
	"strings"
)

// SchemaFor generates a JSON Schema (as a map) from the Go type O using reflection.
//
// It supports:
//   - Basic types: string, int*, uint*, float*, bool
//   - Structs: recursively generates "object" schema with properties
//   - Slices: generates "array" schema
//   - Pointers: unwrapped to underlying type
//   - Maps with string keys: generates "object" with additionalProperties
//
// Struct tags:
//   - `json:"name"`: sets the property name (omitempty, - are respected)
//   - `jsonschema:"description=...,enum=a|b|c"`: adds description and enum constraints
func SchemaFor[O any]() map[string]any {
	var zero O
	t := reflect.TypeOf(zero)
	if t == nil {
		return map[string]any{"type": "object"}
	}
	return typeToSchema(t)
}

func typeToSchema(t reflect.Type) map[string]any {
	// Unwrap pointer.
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	switch t.Kind() {
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return map[string]any{"type": "integer"}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.Slice:
		items := typeToSchema(t.Elem())
		return map[string]any{"type": "array", "items": items}
	case reflect.Map:
		if t.Key().Kind() == reflect.String {
			values := typeToSchema(t.Elem())
			return map[string]any{"type": "object", "additionalProperties": values}
		}
		return map[string]any{"type": "object"}
	case reflect.Struct:
		return structToSchema(t)
	default:
		return map[string]any{"type": "string"}
	}
}

func structToSchema(t reflect.Type) map[string]any {
	properties := map[string]any{}
	var required []string

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		name, omit := jsonFieldName(field)
		if name == "-" {
			continue
		}

		prop := typeToSchema(field.Type)

		// Parse jsonschema tag.
		if tag := field.Tag.Get("jsonschema"); tag != "" {
			parseSchemaTag(tag, prop)
		}

		properties[name] = prop
		if !omit {
			required = append(required, name)
		}
	}

	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// jsonFieldName extracts the JSON field name and omitempty flag from struct tags.
func jsonFieldName(f reflect.StructField) (name string, omitempty bool) {
	tag := f.Tag.Get("json")
	if tag == "" {
		return f.Name, false
	}
	parts := strings.Split(tag, ",")
	name = parts[0]
	if name == "" {
		name = f.Name
	}
	for _, p := range parts[1:] {
		if p == "omitempty" {
			omitempty = true
		}
	}
	return name, omitempty
}

// parseSchemaTag processes `jsonschema:"description=...,enum=a|b|c"` tags.
func parseSchemaTag(tag string, schema map[string]any) {
	parts := strings.Split(tag, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if kv := strings.SplitN(part, "=", 2); len(kv) == 2 {
			key := strings.TrimSpace(kv[0])
			val := strings.TrimSpace(kv[1])
			switch key {
			case "description":
				schema["description"] = val
			case "enum":
				enumVals := strings.Split(val, "|")
				trimmed := make([]any, len(enumVals))
				for i, v := range enumVals {
					trimmed[i] = strings.TrimSpace(v)
				}
				schema["enum"] = trimmed
			}
		}
	}
}
