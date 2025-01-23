package crudder

import "testing"

// Teste para a função isAlphaNumeric
func TestIsAlphaNumeric(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"table123", true},
		{"TableName", true},
		{"123456", true},
		{"table_name", true},
		{"table-name", true},
		{"table name", false},
		{"table@name", false},
		{"", false},
	}

	for _, tt := range tests {
		result := isAlphaNumeric(tt.input)
		if result != tt.expected {
			t.Errorf("isAlphaNumeric(%q) = %v; expected %v", tt.input, result, tt.expected)
		}
	}
}
