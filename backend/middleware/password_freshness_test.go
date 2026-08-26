package middleware

import (
	"database/sql"
	"testing"
	"time"
)

func TestPasswordTokenStale(t *testing.T) {
	base := time.Date(2026, 3, 14, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		issuedAt  time.Time
		changedAt sql.NullTime
		want      bool
	}{
		{
			name:      "never changed keeps token valid",
			issuedAt:  base,
			changedAt: sql.NullTime{},
		},
		{
			name:      "token issued before change is rejected",
			issuedAt:  base.Add(-time.Hour),
			changedAt: sql.NullTime{Time: base, Valid: true},
			want:      true,
		},
		{
			name:      "token issued within the same second survives",
			issuedAt:  base,
			changedAt: sql.NullTime{Time: base, Valid: true},
		},
		{
			name:      "token issued after change survives",
			issuedAt:  base.Add(time.Second),
			changedAt: sql.NullTime{Time: base, Valid: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PasswordTokenStale(tt.issuedAt, tt.changedAt); got != tt.want {
				t.Fatalf("PasswordTokenStale() = %v, want %v", got, tt.want)
			}
		})
	}
}

// 时间戳必须落在整秒上，否则 MySQL DATETIME 的小数秒进位会晚于随后签发 token 的 iat，
// 导致改密后立即重新登录被判为过期。
func TestNewPasswordChangeStampTruncatesToSecond(t *testing.T) {
	stamp := NewPasswordChangeStamp()
	if stamp.Nanosecond() != 0 {
		t.Fatalf("NewPasswordChangeStamp() = %v, want zero nanoseconds", stamp)
	}
}

func TestEnsurePasswordChangedAtColumnRejectsUnknownTable(t *testing.T) {
	if err := EnsurePasswordChangedAtColumn(nil, "licenses"); err == nil {
		t.Fatal("EnsurePasswordChangedAtColumn() = nil, want error for non-account table")
	}
}