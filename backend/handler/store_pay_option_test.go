package handler

import "testing"

func TestBuyerAccountLine(t *testing.T) {
	if buyerAccountLine(nil) != "" {
		t.Fatal("没有账号对象时应留空，避免刷新清掉旧快照")
	}
	if buyerAccountLine(map[string]any{"role": "agent"}) != "" {
		t.Fatal("只有身份、没有名字时不应覆盖旧账号")
	}
	if got := buyerAccountLine(map[string]any{"name": " list168@example.com ", "role": " user "}); got != "list168@example.com\nuser" {
		t.Fatalf("账号行 = %q", got)
	}
}

func TestPickConfiguredPayOption(t *testing.T) {
	options := []payOption{{Code: "easypay:alipay", Label: "支付宝"}, {Code: "easypay:wxpay", Label: "微信"}}
	got, err := pickConfiguredPayOption(options, "")
	if err != nil || got.Code != "easypay:alipay" {
		t.Fatalf("empty request = %+v %v", got, err)
	}
	got, err = pickConfiguredPayOption(options, "easypay:wxpay")
	if err != nil || got.Label != "微信" {
		t.Fatalf("selected = %+v %v", got, err)
	}
	if _, err = pickConfiguredPayOption(options, "missing"); err == nil {
		t.Fatal("unknown method was accepted")
	}
	if _, err = pickConfiguredPayOption(nil, ""); err == nil {
		t.Fatal("empty list was accepted")
	}
}
