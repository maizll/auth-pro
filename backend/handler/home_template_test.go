package handler

import "testing"

func TestValidateHomeTemplateDocument(t *testing.T) {
	if err := validateHomeTemplateDocument([]byte(`{"schemaVersion":1,"hero":{"title":"首页"}}`)); err != nil {
		t.Fatal(err)
	}
	if err := validateHomeTemplateDocument([]byte(`{"schemaVersion":1,"hero":{"title":"首页"},"scripts":[]}`)); err == nil {
		t.Fatal("scripts should be rejected")
	}
	if err := validateHomeTemplateDocument([]byte(`{"schemaVersion":2,"hero":{"title":"首页"}}`)); err == nil {
		t.Fatal("unsupported schema should be rejected")
	}
}
