package handler

import "testing"

func TestNormalizeStoreSettings(t *testing.T) {
	ok, err := normalizeStoreSettings(sourceStoreSettings{ProductAppKey: " store-app_1 ", FreePlanID: " 12 "})
	if err != nil || ok.ProductAppKey != "store-app_1" || ok.FreePlanID != "12" {
		t.Fatalf("normalized=%#v err=%v", ok, err)
	}
	empty, err := normalizeStoreSettings(sourceStoreSettings{})
	if err != nil || empty.ProductAppKey != "" || empty.FreePlanID != "" {
		t.Fatalf("empty=%#v err=%v", empty, err)
	}
	if _, err := normalizeStoreSettings(sourceStoreSettings{ProductAppKey: "bad key"}); err == nil {
		t.Fatal("expected invalid app key")
	}
	if _, err := normalizeStoreSettings(sourceStoreSettings{FreePlanID: "0"}); err == nil {
		t.Fatal("expected invalid plan id")
	}
	if _, err := normalizeStoreSettings(sourceStoreSettings{FreePlanID: "abc"}); err == nil {
		t.Fatal("expected non-numeric plan id")
	}
}
