package handler

import (
	"testing"
)

func TestCanonicalizeAdSlotDropsPopup(t *testing.T) {
	if got := canonicalizeAdSlot("popup"); got != "" {
		t.Fatalf("popup must be retired, got %q", got)
	}
	if got := canonicalizeAdSlot("sidebar"); got != adSlotConsoleSidebar {
		t.Fatalf("sidebar alias → console-sidebar, got %q", got)
	}
	if _, ok := adSlotDefOf("console-home"); !ok {
		t.Fatal("console-home must exist")
	}
	if advertisementLocks["console-home"] == nil {
		t.Fatal("console-home must have a lock")
	}
	if advertisementLocks["popup"] != nil {
		t.Fatal("popup must not be in locks")
	}
}

func TestOnlineSettlementRouteAd(t *testing.T) {
	if got := onlineSettlementRoute("AD202610110001ABCD"); got != "ad" {
		t.Fatalf("route=%q", got)
	}
}

func TestAdOrderStatusLabelNoRefundStates(t *testing.T) {
	if adOrderStatusLabel(adOrderNeedEdit) != "待修改" {
		t.Fatal(adOrderStatusLabel(adOrderNeedEdit))
	}
	if adOrderStatusLabel(adOrderVoided) != "已作废" {
		t.Fatal(adOrderStatusLabel(adOrderVoided))
	}
}
