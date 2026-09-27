package handler

import (
	"testing"
)

func TestResolveCatalogAccess(t *testing.T) {
	t.Setenv("AUTO_PRO_DATA_DIR", t.TempDir())
	savePaidCatalog([]paidCatalogItem{
		{Kind: "plugin", ID: "alipay-f2f", Name: "支付宝当面付", PriceCents: 100, Billing: "one_time", PurchaseOnly: true},
		{Kind: "plugin", ID: "dev-extra", Name: "开发者插件", PriceCents: 5000, Billing: "one_time", PurchaseOnly: true},
	})
	commercial := buyerAccessView{Edition: storeEditionCommercial}
	face := resolveCatalogAccess(commercial, "plugin", "alipay-f2f", 100)
	if face.Party != "official" || !face.CommercialIncluded || !face.Owned || face.Grant != "commercial" {
		t.Fatalf("商业版当面付 = %+v", face)
	}
	if ownershipLabel(100, face) != "included" {
		t.Fatalf("权益标签 = %s", ownershipLabel(100, face))
	}
	dev := resolveCatalogAccess(commercial, "plugin", "dev-extra", 5000)
	if dev.Party != "third" || dev.CommercialIncluded || dev.Owned || dev.Grant != "" {
		t.Fatalf("商业版仍不能白拿第三方 = %+v", dev)
	}
	freeUser := buyerAccessView{Edition: storeEditionFree}
	faceFree := resolveCatalogAccess(freeUser, "plugin", "alipay-f2f", 100)
	if faceFree.Party != "official" || !faceFree.CommercialIncluded || faceFree.Owned || faceFree.Grant != "" {
		t.Fatalf("免费用户当面付应显示可升级、尚未拥有 = %+v", faceFree)
	}
}
