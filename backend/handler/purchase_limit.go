package handler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"auto_pro/config"
)

// purchaseLimitViolation is returned only when an active limit blocks a purchase.
type purchaseLimitViolation struct{ message string }

func (e *purchaseLimitViolation) Error() string { return e.message }

// purchaseLimitViolationMessage maps the domain limit error to a client-safe message.
func purchaseLimitViolationMessage(err error) string {
	var violation *purchaseLimitViolation
	if errors.As(err, &violation) {
		return violation.Error()
	}
	return ""
}

// enforcePurchaseLimit locks the active unified campaign plan and counts completed
// licenses plus unexpired pending online orders. It must run inside the purchase transaction.
func enforcePurchaseLimit(ctx context.Context, tx *sql.Tx, appID, planID int64, buyerType purchaseAudience, ownerType string, ownerID int64) error {
	if ownerID <= 0 || (ownerType != "user" && ownerType != "agent") {
		return fmt.Errorf("授权归属信息无效")
	}
	var perOwnerLimit, stockLimit int
	err := tx.QueryRowContext(ctx, `
		SELECT p.per_owner_limit, p.stock_limit
		FROM promotion_campaigns c
		JOIN promotion_campaign_plans p ON p.campaign_id = c.id
		WHERE c.app_id = ? AND p.plan_id = ?
		  AND c.enabled = 1 AND c.purchase_limit_enabled = 1
		  AND c.starts_at <= NOW() AND c.ends_at > NOW()
		  AND c.audience IN (?, 'all')
		ORDER BY c.starts_at DESC, c.id DESC
		LIMIT 1
		FOR UPDATE
	`, appID, planID, buyerType).Scan(&perOwnerLimit, &stockLimit)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if perOwnerLimit > 0 {
		var completed, pending int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM licenses
			WHERE app_id = ? AND plan_id = ? AND owner_type = ? AND owner_id = ?
			  AND source IN ('agent', 'user_purchase')
		`, appID, planID, ownerType, ownerID).Scan(&completed); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM license_purchase_orders
			WHERE app_id = ? AND plan_id = ? AND owner_type = ? AND owner_id = ?
			  AND status = 'pending' AND (expires_at IS NULL OR expires_at > NOW())
		`, appID, planID, ownerType, ownerID).Scan(&pending); err != nil {
			return err
		}
		if completed+pending >= perOwnerLimit {
			return &purchaseLimitViolation{message: fmt.Sprintf("该活动每个持有方限购 %d 份，当前额度已用尽", perOwnerLimit)}
		}
	}
	if stockLimit > 0 {
		var completed, pending int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM licenses
			WHERE app_id = ? AND plan_id = ? AND source IN ('agent', 'user_purchase')
		`, appID, planID).Scan(&completed); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM license_purchase_orders
			WHERE app_id = ? AND plan_id = ? AND status = 'pending'
			  AND (expires_at IS NULL OR expires_at > NOW())
		`, appID, planID).Scan(&pending); err != nil {
			return err
		}
		if completed+pending >= stockLimit {
			return &purchaseLimitViolation{message: "该活动库存仅剩 0 份，暂时无法购买"}
		}
	}
	return nil
}

const purchaseOrderPendingTTL = 30 * time.Minute

func cancelExpiredLicensePurchaseOrders(db *sql.DB) (int64, error) {
	result, err := db.Exec(`
		UPDATE license_purchase_orders
		SET status = 'cancelled', remark = '支付超时，订单已取消并释放限购额度'
		WHERE status = 'pending' AND expires_at IS NOT NULL AND expires_at <= NOW()
	`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

var purchaseOrderExpiryWorkerOnce sync.Once

// StartPurchaseOrderExpiryWorker periodically cancels expired online purchase orders.
func StartPurchaseOrderExpiryWorker() {
	purchaseOrderExpiryWorkerOnce.Do(func() {
		go func() {
			time.Sleep(10 * time.Second)
			runExpiredLicensePurchaseOrderScan()
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			for range ticker.C {
				runExpiredLicensePurchaseOrderScan()
			}
		}()
	})
}

func runExpiredLicensePurchaseOrderScan() {
	db, err := config.DB()
	if err != nil {
		return
	}
	if err := ensureLicensePurchaseOrderSchema(db); err != nil {
		return
	}
	_, _ = cancelExpiredLicensePurchaseOrders(db)
}
