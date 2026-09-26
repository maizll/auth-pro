package handler

import "testing"

func TestLocalCommercialCacheStale(t *testing.T) {
	if localCommercialCacheStale(false, false, false, false) {
		t.Fatal("远程买方的快照不在本机库里，不能当成已失效")
	}
	if localCommercialCacheStale(true, true, true, true) {
		t.Fatal("绑定、授权和商业版权益都还在时，不应失效")
	}
	if !localCommercialCacheStale(true, true, false, true) {
		t.Fatal("授权已从列表删除时，本机商业版应失效")
	}
	if !localCommercialCacheStale(true, true, true, false) {
		t.Fatal("商业版权益已吊销时，本机商业版应失效")
	}
	if !localCommercialCacheStale(true, false, true, true) {
		t.Fatal("站点绑定已吊销时，本机商业版应失效")
	}
}

func TestSnapshotMatchesCommercialLicense(t *testing.T) {
	state := buyerSnapshotState{LicenseNo: "LIC-1", BindingID: "sb_1"}
	state.Snapshot.LicenseNo = "LIC-1"
	state.Snapshot.BindingID = "sb_1"
	if !snapshotMatchesCommercialLicense(state, "LIC-1", nil) {
		t.Fatal("授权编号应能对上本机快照")
	}
	if !snapshotMatchesCommercialLicense(state, "", []string{"sb_1"}) {
		t.Fatal("绑定编号应能对上本机快照")
	}
	if snapshotMatchesCommercialLicense(state, "LIC-OTHER", []string{"sb_other"}) {
		t.Fatal("别的授权不应清掉当前快照")
	}
}
