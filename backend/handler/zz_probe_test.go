package handler

import (
	"context"
	"testing"
)

func TestProbeRealUpstream(t *testing.T) {
	records, err := fetchAdvertisementsUpstream(context.Background(), "home-banner")
	t.Logf("err=%v records=%d", err, len(records))
	for _, r := range records {
		t.Logf("id=%s title=%s pos=%s weight=%d start=%s end=%s img=%.60s...", r.ID, r.Title, r.Position, r.Weight, r.StartAt, r.EndAt, r.ImageURL)
	}
}
