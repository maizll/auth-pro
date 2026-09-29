package handler

import "testing"

func TestRequireLocalRootRejectsNonRoot(t *testing.T) {
	previous := currentEUID
	currentEUID = func() int { return 1000 }
	t.Cleanup(func() { currentEUID = previous })
	if err := requireLocalRoot(); err == nil {
		t.Fatal("非 root 仍允许重设管理员密码")
	}
}

func TestRequireLocalRootAllowsRoot(t *testing.T) {
	previous := currentEUID
	currentEUID = func() int { return 0 }
	t.Cleanup(func() { currentEUID = previous })
	if err := requireLocalRoot(); err != nil {
		t.Fatal(err)
	}
}

func TestChooseAdminPasswordTarget(t *testing.T) {
	admin, err := chooseAdminPasswordTarget([]adminAccount{{ID: 2, Username: "other"}, {ID: 1, Username: "admin"}})
	if err != nil || admin.ID != 1 || admin.Username != "admin" {
		t.Fatalf("admin=%+v err=%v", admin, err)
	}
	only, err := chooseAdminPasswordTarget([]adminAccount{{ID: 7, Username: "root"}})
	if err != nil || only.ID != 7 {
		t.Fatalf("only=%+v err=%v", only, err)
	}
	if _, err := chooseAdminPasswordTarget(nil); err == nil {
		t.Fatal("空管理员列表没有拒绝")
	}
	if _, err := chooseAdminPasswordTarget([]adminAccount{{ID: 1, Username: "a"}, {ID: 2, Username: "b"}}); err == nil {
		t.Fatal("多个非 admin 账号没有拒绝")
	}
	if _, err := chooseAdminPasswordTarget([]adminAccount{{ID: 1, Username: "admin"}, {ID: 2, Username: "admin"}}); err == nil {
		t.Fatal("多个 admin 没有拒绝")
	}
}

func TestRandomDigitPassword(t *testing.T) {
	password, err := randomDigitPassword(8)
	if err != nil {
		t.Fatal(err)
	}
	if len(password) != 8 {
		t.Fatalf("length=%d password=%q", len(password), password)
	}
	for _, ch := range password {
		if ch < '0' || ch > '9' {
			t.Fatalf("password=%q", password)
		}
	}
}
