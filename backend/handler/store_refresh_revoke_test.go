package handler

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func commercialRefreshState() buyerSnapshotState {
	return buyerSnapshotState{LastRefreshOK: true, ExplicitRevoked: false}
}

func assertRefreshDropsCommercial(t *testing.T, err error, reason string) {
	t.Helper()
	next, save := applyBuyerRefreshFailure(commercialRefreshState(), err)
	if !save || !next.ExplicitRevoked || next.LastRefreshOK {
		t.Fatalf("save=%v revoked=%v ok=%v reason=%s", save, next.ExplicitRevoked, next.LastRefreshOK, next.RevokeReason)
	}
	if reason != "" && next.RevokeReason != reason {
		t.Fatalf("revoke reason=%s", next.RevokeReason)
	}
	now := time.Now()
	tier, got := evaluatePaidAccess(paidAccessInput{
		SnapshotOK: true, Edition: storeEditionCommercial,
		SnapshotDomain: "shop.example.com", RequestDomain: "shop.example.com",
		Now: now, GraceUntil: now.Add(7 * 24 * time.Hour),
		ExplicitRevoked: next.ExplicitRevoked, Offline: !next.LastRefreshOK,
	})
	if tier != storeEditionFree || got != "revoked" {
		t.Fatalf("tier=%s reason=%s", tier, got)
	}
}

func TestBuyerRefreshDeletedLicenseDropsCommercial(t *testing.T) {
	raw := []byte(`{"code":403,"msg":"主授权已失效","data":{"reason":"license_deleted","revoked":true}}`)
	var envelope map[string]any
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	err := sourceErrorFromEnvelope(envelope)
	assertRefreshDropsCommercial(t, err, "license_deleted")
}

func TestBuyerRefreshRevokeDropsCommercialImmediately(t *testing.T) {
	err := sourceErrorFromEnvelope(map[string]any{
		"code": float64(401),
		"msg":  "绑定已失效",
		"data": map[string]any{"reason": "binding_revoked", "revoked": true},
	})
	assertRefreshDropsCommercial(t, err, "binding_revoked")

	coded := sourceErrorFromEnvelope(map[string]any{
		"code":    "license_deleted",
		"msg":     "授权已删除",
		"revoked": true,
	})
	assertRefreshDropsCommercial(t, coded, "license_deleted")
}

func TestBuyerRefreshNetworkErrorKeepsGrace(t *testing.T) {
	next, save := applyBuyerRefreshFailure(commercialRefreshState(), errors.New("无法连接源站"))
	if !save || next.ExplicitRevoked || next.LastRefreshOK {
		t.Fatalf("network save=%v revoked=%v ok=%v", save, next.ExplicitRevoked, next.LastRefreshOK)
	}
	now := time.Now()
	tier, reason := evaluatePaidAccess(paidAccessInput{
		SnapshotOK: true, Edition: storeEditionCommercial,
		SnapshotDomain: "shop.example.com", RequestDomain: "shop.example.com",
		Now: now, GraceUntil: now.Add(7 * 24 * time.Hour),
		ExplicitRevoked: next.ExplicitRevoked, Offline: !next.LastRefreshOK,
	})
	if tier != storeEditionCommercial || reason != "" {
		t.Fatalf("grace tier=%s reason=%s", tier, reason)
	}

	plain, savePlain := applyBuyerRefreshFailure(commercialRefreshState(), errors.New("绑定不存在"))
	if savePlain || plain.ExplicitRevoked || !plain.LastRefreshOK {
		t.Fatal("中文「绑定不存在」不能当成吊销，也不能进离线宽限")
	}
	queryFailed, saveQuery := applyBuyerRefreshFailure(commercialRefreshState(), &sourceResponseError{Code: 500, Msg: "查询主授权失败"})
	if saveQuery || queryFailed.ExplicitRevoked || !queryFailed.LastRefreshOK {
		t.Fatal("查询失败不能当成吊销或离线")
	}
}
