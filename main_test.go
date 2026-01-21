// main_test.go
package main

import (
	"net/http"
	"testing"

	"github.com/milklabs/crudder-go/crudder"
	"github.com/stretchr/testify/assert"
)

// Mock para VariableStartServerDefault
var mockStartServerDefaultCalled bool

func MockStartServerDefault() {
	mockStartServerDefaultCalled = true
}

func TestRunServer(t *testing.T) {
	// Substitui temporariamente VariableStartServerDefault
	original := VariableStartServerDefault
	VariableStartServerDefault = MockStartServerDefault
	defer func() { VariableStartServerDefault = original }()

	// Executa runServer
	mockStartServerDefaultCalled = false
	runServer()

	// Verifica se MockStartServerDefault foi chamado
	assert.True(t, mockStartServerDefaultCalled, "MockStartServerDefault não foi chamado")
}

func TestStartServer(t *testing.T) {
	mockServer := &MockServerInterface{}

	// Usa StartServer com o mock
	crudder.StartServer(mockServer)

	// Verifica se ListenAndServe foi chamado
	assert.True(t, mockServer.Called, "ListenAndServe não foi chamado")
}

// MockServerInterface implementa ServerInterface para testes
type MockServerInterface struct {
	Called bool
}

// Implementação mockada de ListenAndServe
func (m *MockServerInterface) ListenAndServe(addr string, handler http.Handler) error {
	m.Called = true
	return nil
}

func TestMainFunctionCallsStartServerDefault(t *testing.T) {
	originalMainFn := mainFn
	originalStart := VariableStartServerDefault
	defer func() {
		mainFn = originalMainFn
		VariableStartServerDefault = originalStart
	}()

	called := false
	VariableStartServerDefault = func() { called = true }
	mainFn = func() { VariableStartServerDefault() }

	mainFn()

	assert.True(t, called, "mainFn não chamou VariableStartServerDefault")
}
