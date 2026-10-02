package handler

import "testing"

func TestNormalizeStoreSettings(t *testing.T) {
	empty, err := normalizeStoreSettings(sourceStoreSettings{})
	if err != nil || empty.GraceDays != storeGraceDefaultDays || empty.RevokeOnPasswordChange == nil || !*empty.RevokeOnPasswordChange ||
		len(empty.CommercialFeatures) != 1 || empty.CommercialFeatures[0] != storeFeatureMultiApp {
		t.Fatalf("empty=%#v err=%v", empty, err)
	}
	off := false
	ok, err := normalizeStoreSettings(sourceStoreSettings{GraceDays: 3, RevokeOnPasswordChange: &off, CommercialFeatures: []string{" multi_app ", "multi_app", ""}})
	if err != nil || ok.GraceDays != 3 || *ok.RevokeOnPasswordChange || len(ok.CommercialFeatures) != 1 {
		t.Fatalf("normalized=%#v err=%v", ok, err)
	}
	if _, err := normalizeStoreSettings(sourceStoreSettings{GraceDays: 31}); err == nil {
		t.Fatal("expected invalid grace days")
	}
	if _, err := normalizeStoreSettings(sourceStoreSettings{CommercialFeatures: []string{"Bad Key"}}); err == nil {
		t.Fatal("expected invalid feature key")
	}
}
