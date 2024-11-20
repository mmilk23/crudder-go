package crudder

import "testing"

// Teste para a função isAlphaNumeric
func TestIsAlphaNumeric(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"table123", true},    // Alfanumérico válido
		{"TableName", true},   // Alfanumérico com letras maiúsculas
		{"123456", true},      // Apenas números
		{"table_name", false}, // Inválido devido ao sublinhado
		{"table-name", false}, // Inválido devido ao traço
		{"table name", false}, // Inválido devido ao espaço
		{"table@name", false}, // Inválido devido ao símbolo especial
		{"", false},           // Inválido devido à string vazia
	}

	for _, tt := range tests {
		result := isAlphaNumeric(tt.input)
		if result != tt.expected {
			t.Errorf("isAlphaNumeric(%q) = %v; expected %v", tt.input, result, tt.expected)
		}
	}
}
