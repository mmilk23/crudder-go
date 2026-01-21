// main_test.go
package main

import (
	"net/http"
	"testing"

	"github.com/milklabs/crudder-go/crudder"
	"github.com/stretchr/testify/assert"
)

func TestRunServer(t *testing.T) {
	called := false

	original := VariableStartServerDefault
	VariableStartServerDefault = func() { called = true }
	defer func() { VariableStartServerDefault = original }()

	runServer()

	assert.True(t, called, "VariableStartServerDefault não foi chamado via runServer")
}

func TestMainFn(t *testing.T) {
	called := false

	original := VariableStartServerDefault
	VariableStartServerDefault = func() { called = true }
	defer func() { VariableStartServerDefault = original }()

	mainFn()

	assert.True(t, called, "VariableStartServerDefault não foi chamado via mainFn")
}

func TestMain_callsMainFn(t *testing.T) {
	called := false

	originalMainFn := mainFn
	mainFn = func() { called = true }
	defer func() { mainFn = originalMainFn }()

	main()

	assert.True(t, called, "main() não chamou mainFn()")
}

func TestStartServer(t *testing.T) {
	mockServer := &MockServerInterface{}

	crudder.StartServer(mockServer)

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
