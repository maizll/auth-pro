package handler

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"auto_pro/config"
	"auto_pro/middleware"

	"github.com/gin-gonic/gin"
)

var (
	storeRefreshOnce sync.Once
	storeInstallMu   sync.Mutex
	storeInstallJobs = map[string]storeInstallJob{}
)

type storeInstallJob struct {
	Kind      string    `json:"kind"`
	ID        string    `json:"id"`
	Attempts  int       `json:"attempts"`
	NextTry   time.Time `json:"nextTry"`
	LastError string    `json:"lastError"`
}

func RegisterPaidStoreRoutes(engine *gin.Engine, api *gin.RouterGroup) {
	api.GET("/v1/public/station-verify/:nonce", BuyerStationVerify)
	api.GET("/v1/store/captcha", StoreAuthCaptcha)
	api.POST("/v1/store/auth/login", StoreAuthLogin)
	api.POST("/v1/store/auth/confirm", StoreAuthConfirm)
	api.POST("/v1/store/auth/logout", StoreAuthLogout)
	api.POST("/v1/store/auth/rotate", StoreAuthRotate)
	api.GET("/v1/store/status", StoreStatus)
	api.GET("/v1/store/edition-plans", StoreEditionPlans)
	api.POST("/v1/store/orders", StoreOrderCreate)
	api.GET("/v1/store/orders/:orderNo", StoreOrderQuery)
	api.POST("/v1/store/download-ticket", StoreDownloadTicket)
	api.GET("/v1/store/packages/:token", StorePackageDownload)
	engine.GET("/store/pay-complete", StorePayCompletePage)

	admin := api.Group("/v1/source/admin")
	admin.Use(middleware.JWTAuth(), middleware.RequireAdmin())
	registerStoreAdminRoutes(admin)
}

func RegisterBuyerStoreRoutes(group *gin.RouterGroup) {
	group.GET("/store/account", BuyerStoreAccount)
	group.PUT("/store/settings", BuyerStoreSettingsSave)
	group.GET("/store/plans", BuyerStorePlans)
	group.GET("/store/catalog", BuyerStoreCatalog)
	group.POST("/store/bind", BuyerStoreBind)
	group.POST("/store/register/email-code", BuyerStoreRegisterCode)
	group.POST("/store/register", BuyerStoreRegister)
	group.POST("/store/orders", BuyerStoreOrderCreate)
	group.GET("/store/orders/:orderNo", BuyerStoreOrderQuery)
	group.POST("/store/refresh", BuyerStoreRefresh)
	group.POST("/store/install", BuyerStoreInstall)
	group.POST("/store/logout", BuyerStoreLogout)
}

func RegisterPanelStoreRoutes(userGroup, agentGroup *gin.RouterGroup) {
	userGroup.GET("/store/stations", func(c *gin.Context) { panelStoreStations(c, "user") })
	userGroup.GET("/store/purchases", func(c *gin.Context) { panelStorePurchases(c, "user") })
	agentGroup.GET("/store/stations", func(c *gin.Context) { panelStoreStations(c, "agent") })
	agentGroup.GET("/store/purchases", func(c *gin.Context) { panelStorePurchases(c, "agent") })
}

func BuyerStationVerify(c *gin.Context) {
	nonce := strings.TrimSpace(c.Param("nonce"))
	installID := loadBuyerInstallID()
	c.String(http.StatusOK, stationChallengeReceipt(installID, nonce))
}

func BuyerStoreAccount(c *gin.Context) {
	view := currentBuyerAccess(c)
	db, _ := config.DB()
	sourceBase := "https://auth.maizll.com"
	siteURL := ""
	trust := false
	installID := loadBuyerInstallID()
	if db != nil {
		if value := configValue(db, storeConfigGroup, storeConfigSourceBase); value != "" {
			sourceBase = value
		}
		siteURL = configValue(db, storeConfigGroup, storeConfigSiteURL)
		trust = configValue(db, storeConfigGroup, storeConfigTrustProxy) == "1"
	}
	storeData(c, gin.H{
		"bound": view.Bound, "account": view.AccountName, "role": view.AccountRole, "licenseNo": view.LicenseNo,
		"domain": view.Domain, "requestDomain": view.RequestDomain, "domainMismatch": view.DomainMismatch,
		"edition": view.Edition, "editionExpireAt": view.ExpireAt, "permanent": view.Permanent,
		"features": view.Features, "verifiedAt": view.VerifiedAt, "graceUntil": view.GraceUntil,
		"offlineGrace": view.OfflineGrace, "graceWarning": view.GraceWarning, "explicitRevoked": view.ExplicitRevoked,
		"reason": view.Reason, "sourceBase": sourceBase, "siteUrl": siteURL, "trustProxy": trust, "installId": installID,
	})
}

func BuyerStoreSettingsSave(c *gin.Context) {
	var req struct {
		SourceBase string `json:"sourceBase"`
		SiteURL    string `json:"siteUrl"`
		TrustProxy bool   `json:"trustProxy"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		storeFail(c, 400, "参数错误")
		return
	}
	if _, err := parseHTTPSBase(req.SourceBase); err != nil {
		storeFail(c, 400, "源站地址必须是 https")
		return
	}
	if strings.TrimSpace(req.SiteURL) != "" {
		if _, err := parseHTTPSBase(req.SiteURL); err != nil {
			storeFail(c, 400, "站点地址必须是 https")
			return
		}
	}
	db, err := openStoreDB(c)
	if err != nil {
		return
	}
	trust := "0"
	if req.TrustProxy {
		trust = "1"
	}
	if err := upsertConfigValue(db, storeConfigGroup, storeConfigSourceBase, strings.TrimRight(strings.TrimSpace(req.SourceBase), "/"), "买方商店源站根地址"); err != nil {
		storeFail(c, 500, "保存失败")
		return
	}
	_ = upsertConfigValue(db, storeConfigGroup, storeConfigSiteURL, strings.TrimSpace(req.SiteURL), "买方站点公网地址")
	_ = upsertConfigValue(db, storeConfigGroup, storeConfigTrustProxy, trust, "绑定域名是否信任反向代理")
	storeData(c, gin.H{"ok": true})
}

func BuyerStorePlans(c *gin.Context) {
	var payload map[string]any
	if err := callSourceJSON(http.MethodGet, "/api/v1/store/edition-plans", nil, nil, &payload); err != nil {
		storeFail(c, 400, err.Error())
		return
	}
	c.JSON(http.StatusOK, payload)
}

func BuyerStoreCatalog(c *gin.Context) {
	view := currentBuyerAccess(c)
	items := loadPaidCatalog()
	out := make([]gin.H, 0, len(items))
	for _, item := range items {
		out = append(out, gin.H{
			"kind": item.Kind, "id": item.ID, "name": item.Name, "version": item.Version, "priceCents": item.PriceCents,
			"ownership": ownershipForPrice(item.PriceCents, view.Edition == storeEditionCommercial, buyerItemEntitled(view, item.Kind, item.ID)),
		})
	}
	storeData(c, gin.H{"list": out, "edition": view.Edition})
}

func BuyerStoreBind(c *gin.Context) {
	var req map[string]any
	body, _ := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if json.Unmarshal(body, &req) != nil {
		storeFail(c, 400, "参数错误")
		return
	}
	delete(req, "password")
	var incoming struct {
		Account  string `json:"account"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	_ = json.Unmarshal(body, &incoming)
	domain := buyerRequestDomain(c)
	if err := rejectStoreDomain(c.Request.Context(), domain); err != nil {
		storeFail(c, 400, err.Error())
		return
	}
	loginBody := map[string]any{}
	_ = json.Unmarshal(body, &loginBody)
	loginBody["domain"] = domain
	loginBody["installId"] = loadBuyerInstallID()
	loginBody["appVersion"] = config.AppVersion
	var login map[string]any
	if err := callSourceJSON(http.MethodPost, "/api/v1/store/auth/login", loginBody, nil, &login); err != nil {
		storeFail(c, 400, err.Error())
		return
	}
	incoming.Password = ""
	data, _ := login["data"].(map[string]any)
	challengeID, _ := data["challengeId"].(string)
	var confirmed map[string]any
	if err := callSourceJSON(http.MethodPost, "/api/v1/store/auth/confirm", map[string]any{"challengeId": challengeID}, nil, &confirmed); err != nil {
		storeFail(c, 400, err.Error())
		return
	}
	if err := persistBuyerBind(confirmed); err != nil {
		storeFail(c, 500, err.Error())
		return
	}
	BuyerStoreAccount(c)
}

func BuyerStoreRegisterCode(c *gin.Context) {
	proxySourcePanel(c, "/api/user-panel/register/email-code")
}
func BuyerStoreRegister(c *gin.Context) { proxySourcePanel(c, "/api/user-panel/register") }

func BuyerStoreOrderCreate(c *gin.Context) {
	var req struct {
		PlanID int64 `json:"planId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.PlanID <= 0 {
		storeFail(c, 400, "请选择商业版套餐")
		return
	}
	var payload map[string]any
	if err := signedSourceJSON(http.MethodPost, "/api/v1/store/orders", map[string]any{"itemKind": "edition", "planId": req.PlanID}, &payload); err != nil {
		storeFail(c, 400, err.Error())
		return
	}
	c.JSON(http.StatusOK, payload)
}

func BuyerStoreOrderQuery(c *gin.Context) {
	orderNo := strings.TrimSpace(c.Param("orderNo"))
	var payload map[string]any
	if err := signedSourceJSON(http.MethodGet, "/api/v1/store/orders/"+orderNo, nil, &payload); err != nil {
		storeFail(c, 400, err.Error())
		return
	}
	if data, ok := payload["data"].(map[string]any); ok {
		if snap, ok := data["snapshot"].(map[string]any); ok {
			_ = saveSnapshotMap(snap, true, false, "")
		}
	}
	c.JSON(http.StatusOK, payload)
}

func BuyerStoreRefresh(c *gin.Context) {
	if err := refreshBuyerSnapshot(c.Request.Context(), buyerRequestDomain(c)); err != nil {
		storeFail(c, 400, err.Error())
		return
	}
	BuyerStoreAccount(c)
}

func BuyerStoreLogout(c *gin.Context) {
	_ = signedSourceJSON(http.MethodPost, "/api/v1/store/auth/logout", map[string]any{}, nil)
	_ = os.Remove(buyerSnapshotPath())
	_ = os.Remove(filepath.Join(config.GetDataDir(), "store", "binding.key"))
	storeData(c, gin.H{"ok": true})
}

func BuyerStoreInstall(c *gin.Context) {
	var req struct {
		Kind string `json:"kind"`
		ID   string `json:"id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || (req.Kind != "plugin" && req.Kind != "template") || strings.TrimSpace(req.ID) == "" {
		storeFail(c, 400, "参数错误")
		return
	}
	if err := installPaidPackage(c.Request.Context(), req.Kind, req.ID); err != nil {
		enqueueInstall(req.Kind, req.ID, err.Error())
		storeFail(c, 400, err.Error())
		return
	}
	storeData(c, gin.H{"ok": true})
}

func panelStoreStations(c *gin.Context, role string) {
	ownerID, ok := panelOwnerID(c, role)
	if !ok {
		return
	}
	db, err := openStoreDB(c)
	if err != nil {
		return
	}
	rows, err := db.Query(`SELECT binding_id, domain_snapshot, status, created_at, last_seen_at FROM store_bindings WHERE owner_type = ? AND owner_id = ? ORDER BY id DESC`, role, ownerID)
	if err != nil {
		storeFail(c, 500, "读取绑定站点失败")
		return
	}
	defer rows.Close()
	list := make([]gin.H, 0)
	for rows.Next() {
		var id, domain, status string
		var created, seen sql.NullTime
		if err := rows.Scan(&id, &domain, &status, &created, &seen); err != nil {
			continue
		}
		item := gin.H{"bindingId": id, "domain": domain, "status": status}
		if created.Valid {
			item["createdAt"] = created.Time.Format("2006-01-02 15:04:05")
		}
		list = append(list, item)
	}
	storeData(c, gin.H{"list": list})
}

func panelStorePurchases(c *gin.Context, role string) {
	ownerID, ok := panelOwnerID(c, role)
	if !ok {
		return
	}
	db, err := openStoreDB(c)
	if err != nil {
		return
	}
	rows, err := db.Query(`SELECT order_no, item_kind, title_snapshot, amount_cents, status, paid_at FROM store_purchase_orders WHERE owner_type = ? AND owner_id = ? ORDER BY id DESC`, role, ownerID)
	if err != nil {
		storeFail(c, 500, "读取已购项目失败")
		return
	}
	defer rows.Close()
	list := make([]gin.H, 0)
	for rows.Next() {
		var orderNo, kind, title, status string
		var amount int64
		var paid sql.NullTime
		if err := rows.Scan(&orderNo, &kind, &title, &amount, &status, &paid); err != nil {
			continue
		}
		item := gin.H{"orderNo": orderNo, "itemKind": kind, "title": title, "amountCents": amount, "status": status}
		if paid.Valid {
			item["paidAt"] = paid.Time.Format("2006-01-02 15:04:05")
		}
		list = append(list, item)
	}
	storeData(c, gin.H{"list": list})
}
