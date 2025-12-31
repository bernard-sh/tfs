package web

import (
	"os"
	"strings"
	"testing"
)

func TestGenerateHTML(t *testing.T) {
	// Mock plan data
	plan := map[string]interface{}{
		"format_version": "0.1",
		"resource_changes": []map[string]interface{}{
			{
				"address": "test_resource",
				"type":    "test_type",
				"name":    "test_name",
				"change": map[string]interface{}{
					"actions": []string{"create"},
				},
			},
		},
	}

	outputPath := "test_output.html"
	defer os.Remove(outputPath) // Cleanup after test

	err := GenerateHTML(plan, outputPath)
	if err != nil {
		t.Fatalf("GenerateHTML failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Fatalf("Output file was not created at %s", outputPath)
	}

	// Verify content
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	htmlStr := string(content)

	// Check for basic HTML structure
	if !strings.Contains(htmlStr, "<!DOCTYPE html>") {
		t.Errorf("HTML output missing doctype")
	}

	// Check if plan data is embedded
	// json.Marshal escapes HTML chars, but basic keys should differ.
	// We expect "test_resource" to be present in the embedded JSON
	if !strings.Contains(htmlStr, "test_resource") {
		t.Errorf("HTML output does not contain plan data")
	}
}
