// 买家站的商业版接口：绑定账号、看套餐和目录、下单、查单、刷新快照、安装付费包。源站根地址不在这里保存。

package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
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

// RegisterPaidStoreRoutes 注册源站公开的商店路由：登录、下单、下载和管理端补发。
// engine 用来挂支付完成页；api 是 /api 路由组。不读取买家本机配置。
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
	registerCommercialReissueRoutes(admin)
}

// RegisterBuyerStoreRoutes 注册买家站 /api/store 下的账号、绑定、下单、刷新和安装。
// 这些接口再去请求固定的源站。参数错误或源站拒绝时返回业务码 400；绑定写盘失败返回 500。
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

// RegisterPanelStoreRoutes 给用户端和代理端挂上「已绑定站点」和「购买记录」。
// role 由分组决定，调用方必须已经登录。找不到账号时由内部函数写错误响应。
func RegisterPanelStoreRoutes(userGroup, agentGroup *gin.RouterGroup) {
	userGroup.GET("/store/stations", func(c *gin.Context) { panelStoreStations(c, "user") })
	userGroup.GET("/store/purchases", func(c *gin.Context) { panelStorePurchases(c, "user") })
	agentGroup.GET("/store/stations", func(c *gin.Context) { panelStoreStations(c, "agent") })
	agentGroup.GET("/store/purchases", func(c *gin.Context) { panelStorePurchases(c, "agent") })
}

// BuyerStationVerify 回应源站的域名挑战。
// 路径里的 nonce 原样参与回执，证明这次请求打到了持有安装编号的买家站。
func BuyerStationVerify(c *gin.Context) {
	nonce := strings.TrimSpace(c.Param("nonce"))
	installID := loadBuyerInstallID()
	c.String(http.StatusOK, stationChallengeReceipt(installID, nonce))
}

// BuyerStoreAccount 返回本机快照里的商业版状态、域名和连接问题。
// 没有快照时仍返回未绑定，不向源站现查。
func BuyerStoreAccount(c *gin.Context) {
	storeData(c, buyerAccountPayload(currentBuyerAccess(c), buyerConnectionForRequest(c), loadBuyerInstallID()))
}

func buyerAccountPayload(view buyerAccessView, conn buyerConnectionView, installID string) gin.H {
	issues := conn.Issues
	if issues == nil {
		issues = []buyerConnectionIssue{}
	}
	return gin.H{
		"bound": view.Bound, "account": view.AccountName, "role": view.AccountRole, "licenseNo": view.LicenseNo,
		"domain": view.Domain, "requestDomain": view.RequestDomain, "domainMismatch": view.DomainMismatch,
		"edition": view.Edition, "editionExpireAt": view.ExpireAt, "permanent": view.Permanent,
		"features": view.Features, "verifiedAt": view.VerifiedAt, "graceUntil": view.GraceUntil,
		"offlineGrace": view.OfflineGrace, "graceWarning": view.GraceWarning, "explicitRevoked": view.ExplicitRevoked,
		"reason": view.Reason, "siteUrl": conn.SiteURL, "trustProxy": conn.TrustProxy,
		"connectionIssues": issues, "installId": installID,
	}
}

// BuyerStoreSettingsSave 保留旧的保存地址，但不再写入源站根、站点地址或信任代理。
// 正式程序的购买网站是固定的，手填会被忽略。请求体格式不对时返回 400。
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
	// 源站根只由环境变量或数据目录文件覆盖。这里忽略 sourceBase，也不再保存手填域名和信任代理。
	_ = req.SourceBase
	_ = req.SiteURL
	_ = req.TrustProxy
	storeData(c, gin.H{"ok": true})
}

// BuyerStorePlans 向源站拉取可购买的商业版套餐。
// 源站不可达或拒绝时返回 400，文案用源站返回的原因。
func BuyerStorePlans(c *gin.Context) {
	var payload map[string]any
	if err := callSourceJSON(http.MethodGet, "/api/v1/store/edition-plans", nil, nil, &payload); err != nil {
		storeFail(c, 400, err.Error())
		return
	}
	c.JSON(http.StatusOK, payload)
}

// BuyerStoreCatalog 返回本机记住的付费插件和模板，并标出当前账号是否已拥有。
// 不访问源站。商业版不包含的条目会标成仅单买。
func BuyerStoreCatalog(c *gin.Context) {
	view := currentBuyerAccess(c)
	items := loadPaidCatalog()
	out := make([]gin.H, 0, len(items))
	for _, item := range items {
		out = append(out, gin.H{
			"kind": item.Kind, "id": item.ID, "name": item.Name, "version": item.Version, "priceCents": item.PriceCents,
			"billing": item.Billing, "purchaseOnly": item.PurchaseOnly || catalogPurchaseOnly(item.Kind, item.ID),
			"ownership": buyerCatalogOwnership(view, item.Kind, item.ID, item.PriceCents),
		})
	}
	storeData(c, gin.H{"list": out, "edition": view.Edition})
}

// BuyerStoreBind 用账号密码向源站换绑定，并写入本机快照。
// 请求体要有账号、密码和身份。域名不合法、登录失败或确认失败返回 400；快照写盘失败返回 500。
// 密码只用于这一次登录，确认成功后不再留在本机。
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

// BuyerStoreRegisterCode 把注册验证码请求原样转到源站用户端。
func BuyerStoreRegisterCode(c *gin.Context) {
	proxySourcePanel(c, "/api/user-panel/register/email-code")
}

// BuyerStoreRegister 把注册请求原样转到源站用户端。
func BuyerStoreRegister(c *gin.Context) { proxySourcePanel(c, "/api/user-panel/register") }

func buyerStoreOrderBody(planID int64, itemKind, itemID string) (map[string]any, error) {
	switch itemKind {
	case "plugin", "template":
		itemID = strings.TrimSpace(itemID)
		if itemID == "" {
			if itemKind == "template" {
				return nil, errors.New("请选择要购买的模板")
			}
			return nil, errors.New("请选择要购买的插件")
		}
		return map[string]any{"itemKind": itemKind, "itemId": itemID}, nil
	default:
		if planID <= 0 {
			return nil, errors.New("请选择商业版套餐")
		}
		return map[string]any{"itemKind": "edition", "planId": planID}, nil
	}
}

// BuyerStoreOrderCreate 向源站下商业版套餐或单品订单。
// itemKind 为 plugin 或 template 时必须有 itemId；否则必须有 planId。
// 源站明确吊销绑定时返回 400，并带 rebind，前端据此回到绑定步骤。其它失败也是 400。
func BuyerStoreOrderCreate(c *gin.Context) {
	var req struct {
		PlanID   int64  `json:"planId"`
		ItemKind string `json:"itemKind"`
		ItemID   string `json:"itemId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		storeFail(c, 400, "参数错误")
		return
	}
	body, err := buyerStoreOrderBody(req.PlanID, req.ItemKind, req.ItemID)
	if err != nil {
		storeFail(c, 400, err.Error())
		return
	}
	var payload map[string]any
	if err := signedSourceJSON(http.MethodPost, "/api/v1/store/orders", body, &payload); err != nil {
		if storeFailSource(c, err) {
			return
		}
		storeFail(c, 400, err.Error())
		return
	}
	c.JSON(http.StatusOK, payload)
}

// BuyerStoreOrderQuery 按订单号向源站查单。
// 响应里如果带了新快照就写到本机，这样付完款后顶栏不用再等下一次定时刷新。
// 绑定已被源站删除时返回 400 和 rebind。
func BuyerStoreOrderQuery(c *gin.Context) {
	orderNo := strings.TrimSpace(c.Param("orderNo"))
	var payload map[string]any
	if err := signedSourceJSON(http.MethodGet, "/api/v1/store/orders/"+orderNo, nil, &payload); err != nil {
		if storeFailSource(c, err) {
			return
		}
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

// BuyerStoreRefresh 立即向源站核对快照并返回最新账号状态。
// 明确吊销时清本地绑定并要求重新绑定；网络失败只返回 400，不把商业版直接降成免费版。
func BuyerStoreRefresh(c *gin.Context) {
	err := refreshBuyerSnapshot(c.Request.Context(), buyerRequestDomain(c))
	if err != nil {
		if storeFailSource(c, err) {
			return
		}
		storeFail(c, 400, err.Error())
		return
	}
	BuyerStoreAccount(c)
}

func buyerRefreshFailureRevoked(err error) bool {
	var src *sourceResponseError
	return errors.As(err, &src) && buyerSnapshotTerminal(src.Reason, src.Revoked)
}

const buyerRebindMessage = "之前的绑定已在源站删除，请重新绑定账号后继续购买"

// storeFailSource 在源站明确终止绑定时返回重新绑定原因。
// 业务码用 400：401 会被前端当成管理员登录失效并退出后台。
func storeFailSource(c *gin.Context, err error) bool {
	var src *sourceResponseError
	if !errors.As(err, &src) || !buyerSnapshotTerminal(src.Reason, src.Revoked) {
		return false
	}
	reason := strings.TrimSpace(src.Reason)
	if reason == "" {
		reason = "revoked"
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 400,
		"msg":  buyerRebindMessage,
		"data": gin.H{"reason": reason, "revoked": true, "rebind": true},
	})
	return true
}

// BuyerStoreLogout 通知源站退出，并清掉本机绑定和快照。
// 源站暂时连不上也会清本地，避免界面还显示已经不想用的商业版。
func BuyerStoreLogout(c *gin.Context) {
	_ = signedSourceJSON(http.MethodPost, "/api/v1/store/auth/logout", map[string]any{}, nil)
	clearBuyerLocalBinding()
	storeData(c, gin.H{"ok": true})
}

// BuyerStoreInstall 按 kind 和 id 把已购买的插件或模板装到本机。
// kind 只能是 plugin 或 template。源站吊销绑定返回 400 和 rebind。
// 下载失败会记下重试，同时把这次的原因返回给页面。
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
		if storeFailSource(c, err) {
			return
		}
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
