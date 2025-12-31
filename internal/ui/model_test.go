package ui

import (
	"testing"
)

func TestGetSymbol(t *testing.T) {
	tests := []struct {
		action   string
		expected string
	}{
		{"create", "+"},
		{"delete", "-"},
		{"update", "~"},
		{"replace", "-/+"},
		{"unknown", ""},
	}

	for _, tt := range tests {
		got := getSymbol(tt.action)
		if got != tt.expected {
			t.Errorf("getSymbol(%q) = %q; want %q", tt.action, got, tt.expected)
		}
	}
}

func TestFormatValue(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string // Partial match check usually easier for complex strings
	}{
		{"String", "hello", "\"hello\""},
		{"Int", 123, "123"},
		{"Nil", nil, "null"},
		{"List", []interface{}{"a", "b"}, "[\n  \"a\",\n  \"b\",\n]"},
	}

	for _, tt := range tests {
		got := formatValue(tt.input, 0)
		if got != tt.expected {
			t.Errorf("formatValue(%s) = %q; want %q", tt.name, got, tt.expected)
		}
	}
}

func TestInitialModel_ValidJSON(t *testing.T) {
	jsonContent := `{
		"resource_changes": [
			{
				"address": "res.create",
				"type": "res",
				"name": "create",
				"change": { "actions": ["create"] }
			},
			{
				"address": "res.delete",
				"type": "res",
				"name": "delete",
				"change": { "actions": ["delete"] }
			},
			{
				"address": "res.replace",
				"type": "res",
				"name": "replace",
				"change": { "actions": ["delete", "create"] }
			}
		]
	}`

	m, err := InitialModel(jsonContent)
	if err != nil {
		t.Fatalf("InitialModel failed: %v", err)
	}

	uiModel, ok := m.(model)
	if !ok {
		t.Fatalf("Returned model is not of type ui.model")
	}

	// Check bucketing
	// 0: Create, 1: Destroy, 2: Replace, 3: Update, 4: Import
	
	if len(uiModel.lists[0]) != 1 {
		t.Errorf("Expected 1 create, got %d", len(uiModel.lists[0]))
	}
	if len(uiModel.lists[1]) != 1 {
		t.Errorf("Expected 1 delete, got %d", len(uiModel.lists[1]))
	}
	if len(uiModel.lists[2]) != 1 {
		t.Errorf("Expected 1 replace, got %d", len(uiModel.lists[2]))
	}
}

func TestInitialModel_InvalidJSON(t *testing.T) {
	jsonContent := `INVALID JSON`
	_, err := InitialModel(jsonContent)
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}
