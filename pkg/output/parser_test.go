package output

import (
	"testing"
)

func TestJSONParser_DirectJSON(t *testing.T) {
	type Person struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	p := NewJSONParser[Person]()

	result, err := p.Parse(`{"name": "Alice", "age": 30}`)
	if err != nil {
		t.Fatal(err)
	}
	if result.Name != "Alice" || result.Age != 30 {
		t.Errorf("unexpected: %+v", result)
	}
}

func TestJSONParser_CodeBlock(t *testing.T) {
	type Item struct {
		ID string `json:"id"`
	}
	p := NewJSONParser[Item]()

	raw := "Here is the result:\n```json\n{\"id\": \"abc\"}\n```\nDone."
	result, err := p.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != "abc" {
		t.Errorf("unexpected: %+v", result)
	}
}

func TestJSONParser_ExtractBracketed(t *testing.T) {
	type Data struct {
		Value int `json:"value"`
	}
	p := NewJSONParser[Data]()

	raw := "The answer is {\"value\": 42} and that's it."
	result, err := p.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if result.Value != 42 {
		t.Errorf("unexpected: %+v", result)
	}
}

func TestJSONParser_Array(t *testing.T) {
	p := NewJSONParser[[]string]()

	raw := "Here: [\"a\", \"b\", \"c\"] done"
	result, err := p.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 3 || result[0] != "a" {
		t.Errorf("unexpected: %+v", result)
	}
}

func TestJSONParser_NestedBraces(t *testing.T) {
	type Nested struct {
		Inner struct {
			Key string `json:"key"`
		} `json:"inner"`
	}
	p := NewJSONParser[Nested]()

	raw := `Some text {"inner": {"key": "val"}} more text`
	result, err := p.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if result.Inner.Key != "val" {
		t.Errorf("unexpected: %+v", result)
	}
}

func TestJSONParser_InvalidJSON(t *testing.T) {
	p := NewJSONParser[struct{ X int }]()
	_, err := p.Parse("This is not JSON at all")
	if err == nil {
		t.Fatal("expected error for non-JSON input")
	}
}

func TestJSONParser_FormatInstruction(t *testing.T) {
	type Review struct {
		Score int    `json:"score"`
		Text  string `json:"text"`
	}
	p := NewJSONParser[Review]()
	inst := p.FormatInstruction()
	if inst == "" {
		t.Fatal("FormatInstruction should not be empty")
	}
	if !contains(inst, "JSON") {
		t.Error("FormatInstruction should mention JSON")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && searchString(s, sub)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
