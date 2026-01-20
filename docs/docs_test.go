package docs

import "testing"

func TestSwaggerInfo_IsInitialized(t *testing.T) {
	if SwaggerInfo == nil {
		t.Fatalf("SwaggerInfo must not be nil")
	}
	if SwaggerInfo.InfoInstanceName == "" {
		t.Fatalf("SwaggerInfo.InfoInstanceName must not be empty")
	}
	if SwaggerInfo.SwaggerTemplate == "" {
		t.Fatalf("SwaggerInfo.SwaggerTemplate must not be empty")
	}
}
