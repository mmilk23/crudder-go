package crudder

import (
	"testing"

	"bou.ke/monkey"
)

func TestStartServerDefault_CallsStartServerWithRealServer(t *testing.T) {
	var called bool
	var got ServerInterface

	patch := monkey.Patch(StartServer, func(s ServerInterface) {
		called = true
		got = s
	})
	defer patch.Unpatch()

	StartServerDefault()

	if !called {
		t.Fatalf("expected StartServer to be called")
	}
	if _, ok := got.(RealServer); !ok {
		t.Fatalf("expected RealServer, got %T", got)
	}
}
