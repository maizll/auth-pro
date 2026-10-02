package handler

import (
	"testing"
	"time"
)

func TestSalePeriodAndEditionExpiry(t *testing.T) {
	if salePeriodFromDuration(0) != storePeriodPermanent || salePeriodFromDuration(365) != storePeriodYearly || salePeriodFromDuration(30) != "d30" {
		t.Fatalf("periods %s %s %s", salePeriodFromDuration(0), salePeriodFromDuration(365), salePeriodFromDuration(30))
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if nextEditionExpiry(storePeriodPermanent, nil, now) != nil {
		t.Fatal("permanent expiry")
	}
	yearly := nextEditionExpiry(storePeriodYearly, nil, now)
	if yearly == nil || !yearly.Equal(now.AddDate(0, 0, 365)) {
		t.Fatalf("yearly=%v", yearly)
	}
	days := nextEditionExpiry("d30", nil, now)
	if days == nil || !days.Equal(now.AddDate(0, 0, 30)) {
		t.Fatalf("d30=%v", days)
	}
	monthly := nextEditionExpiry(storePeriodMonthly, nil, now)
	if monthly == nil || !monthly.Equal(now.AddDate(0, 1, 0)) {
		t.Fatalf("monthly=%v", monthly)
	}
}
