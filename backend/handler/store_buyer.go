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
	api.POST("/v1/store/register/email-code", StoreRegisterEmailCode)
	api.POST("/v1/store/register", StoreRegister)
	api.POST("/v1/store/auth/login", StoreAuthLogin)
	api.POST("/v1/store/auth/confirm", StoreAuthConfirm)
	api.POST("/v1/store/auth/logout", StoreAuthLogout)
	api.POST("/v1/store/auth/handoff", StoreLoginHandoffIssue)
	api.POST("/v1/store/auth/handoff/consume", StoreLoginHandoffConsume)
	api.POST("/v1/store/auth/rotate", StoreAuthRotate)
	api.GET("/v1/store/status", StoreStatus)
	api.GET("/v1/store/binding", StoreBindingCheck)
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
	group.GET("/store/register/captcha", BuyerStoreRegisterCaptcha)
	group.POST("/store/register/email-code", BuyerStoreRegisterCode)
	group.POST("/store/register", BuyerStoreRegister)
	group.POST("/store/orders", BuyerStoreOrderCreate)
	group.GET("/store/orders/:orderNo", BuyerStoreOrderQuery)
	group.POST("/store/refresh", BuyerStoreRefresh)
	group.POST("/store/install", BuyerStoreInstall)
	group.POST("/store/logout", BuyerStoreLogout)
	group.POST("/store/manage-link", BuyerStoreManageLink)
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

// BuyerStoreAccount 返回本机商业版状态。
// 不带 verify 时只读本地快照，供顶栏快速显示。
// verify=1 时向源站核对绑定：源站确认有效才标 sourceVerified。
// 绑定不存在、已删除或令牌失效则清掉本地旧记录，返回未绑定和 bindingInvalid。
// 连不上源站时保留本地快照，不把商业版降成未绑定。
func BuyerStoreAccount(c *gin.Context) {
	invalidReason, verified := "", false
	if c.Query("verify") == "1" || c.Query("verify") == "true" {
		invalidReason, verified = reconcileBuyerBinding()
	}
	writeBuyerAccountResult(c, invalidReason, verified)
}

func writeBuyerAccountResult(c *gin.Context, invalidReason string, verified bool) {
	payload := buyerAccountPayload(currentBuyerAccess(c), buyerConnectionForRequest(c), loadBuyerInstallID())
	payload["sourceVerified"] = verified && invalidReason == ""
	payload["bindingInvalid"] = false
	payload["bindingInvalidReason"] = ""
	if invalidReason != "" {
		payload["bound"] = false
		payload["explicitRevoked"] = false
		payload["edition"] = storeEditionFree
		payload["permanent"] = false
		payload["editionExpireAt"] = nil
		payload["features"] = []string{}
		payload["offlineGrace"] = false
		payload["bindingInvalid"] = true
		payload["bindingInvalidReason"] = invalidReason
		payload["sourceVerified"] = false
		if reason, _ := payload["reason"].(string); buyerSnapshotTerminal(reason, false) {
			payload["reason"] = "unbound"
		}
	}
	storeData(c, payload)
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
	code, err := finishBuyerBind(c, incoming.Account, incoming.Password, incoming.Role)
	incoming.Password = ""
	if err != nil {
		storeFail(c, code, err.Error())
		return
	}
	writeFreshBuyerBind(c)
}

// finishBuyerBind 用账号密码走源站登录和确认。密码只放在这一次请求里，不写入快照，也不打日志。
func finishBuyerBind(c *gin.Context, account, password, role string) (int, error) {
	defer func() { password = "" }()
	domain := buyerRequestDomain(c)
	if err := rejectStoreDomain(c.Request.Context(), domain); err != nil {
		return 400, err
	}
	loginBody := map[string]any{
		"account": account, "password": password, "role": role,
		"domain": domain, "installId": loadBuyerInstallID(), "appVersion": config.AppVersion,
	}
	var login map[string]any
	err := callSourceJSON(http.MethodPost, "/api/v1/store/auth/login", loginBody, nil, &login)
	delete(loginBody, "password")
	if err != nil {
		return 400, err
	}
	data, _ := login["data"].(map[string]any)
	challengeID, _ := data["challengeId"].(string)
	var confirmed map[string]any
	if err := callSourceJSON(http.MethodPost, "/api/v1/store/auth/confirm", map[string]any{"challengeId": challengeID}, nil, &confirmed); err != nil {
		return 400, err
	}
	if err := persistBuyerBind(confirmed, account, role); err != nil {
		return 500, err
	}
	return 200, nil
}

// writeFreshBuyerBind 登录确认已经通过源站。随后的核对若只是网络抖动，仍视为刚刚绑定成功。
func writeFreshBuyerBind(c *gin.Context) {
	invalidReason, verified := reconcileBuyerBinding()
	if invalidReason == "" {
		verified = true
	}
	writeBuyerAccountResult(c, invalidReason, verified)
}

// BuyerStoreRegisterCaptcha 告诉购买窗口发验证码前要不要做行为验证。
// 老源站没有商店验证码接口时，改读官网公开配置里的同一套极验开关。
func BuyerStoreRegisterCaptcha(c *gin.Context) {
	envelope, err := exchangeSource(http.MethodGet, "/api/v1/store/captcha", nil)
	if errors.Is(err, errSourcePathMissing) {
		enabled, captchaID, ok := buyerLegacyRegisterCaptcha()
		if !ok {
			storeData(c, gin.H{"enabled": false, "captchaId": ""})
			return
		}
		storeData(c, gin.H{"enabled": enabled, "captchaId": captchaID})
		return
	}
	if err != nil {
		storeFail(c, 400, plainRegisterMessage(err.Error(), ""))
		return
	}
	c.JSON(http.StatusOK, envelope)
}

func buyerLegacyRegisterCaptcha() (bool, string, bool) {
	var payload map[string]any
	if err := callSourceJSON(http.MethodGet, "/api/system-config/public", nil, nil, &payload); err != nil {
		return false, "", false
	}
	data, _ := payload["data"].(map[string]any)
	if data == nil {
		return false, "", false
	}
	enabled, _ := data["geetestEnabled"].(bool)
	captchaID, _ := data["geetestCaptchaId"].(string)
	captchaID = strings.TrimSpace(captchaID)
	return enabled && captchaID != "", captchaID, true
}

// BuyerStoreRegisterCode 把发验证码转到源站。先走商店接口，老源站没有时再走官网发信。
func BuyerStoreRegisterCode(c *gin.Context) {
	var req struct {
		Email         string `json:"email"`
		LotNumber     string `json:"lot_number"`
		CaptchaOutput string `json:"captcha_output"`
		PassToken     string `json:"pass_token"`
		GenTime       string `json:"gen_time"`
	}
	body, _ := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if json.Unmarshal(body, &req) != nil {
		storeFail(c, 400, "请输入有效的邮箱地址")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if !registerEmailOK(email) {
		storeFail(c, 400, "请输入有效的邮箱地址")
		return
	}
	payload := map[string]any{"email": email}
	if strings.TrimSpace(req.LotNumber) != "" {
		payload["lot_number"] = req.LotNumber
		payload["captcha_output"] = req.CaptchaOutput
		payload["pass_token"] = req.PassToken
		payload["gen_time"] = req.GenTime
	}
	envelope, err := forwardBuyerRegister("/api/v1/store/register/email-code", "/api/user-panel/register/email-code", payload)
	if err != nil {
		storeFail(c, 400, plainRegisterMessage(err.Error(), ""))
		return
	}
	c.JSON(http.StatusOK, envelope)
}

// BuyerStoreRegister 在源站注册，成功后立刻用新邮箱完成绑定。
// 密码只出现在发往源站的这一次请求里，不写入本机，也不打日志。
func BuyerStoreRegister(c *gin.Context) {
	var req struct {
		Email     string `json:"email"`
		EmailCode string `json:"emailCode"`
		Nickname  string `json:"nickname"`
		Password  string `json:"password"`
		Phone     string `json:"phone"`
	}
	body, _ := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if json.Unmarshal(body, &req) != nil {
		storeFail(c, 400, "请检查邮箱、验证码和密码（至少 6 位）")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	emailCode := strings.TrimSpace(req.EmailCode)
	nickname := strings.TrimSpace(req.Nickname)
	password := strings.TrimSpace(req.Password)
	phone := strings.TrimSpace(req.Phone)
	defer func() { password = "" }()
	if msg := registerInputProblem(email, emailCode, nickname, password, phone); msg != "" {
		storeFail(c, 400, msg)
		return
	}
	// 域名不合法时不要先把账号注册出来，否则用户有了号却绑不上。
	if err := rejectStoreDomain(c.Request.Context(), buyerRequestDomain(c)); err != nil {
		storeFail(c, 400, err.Error())
		return
	}
	payload := map[string]any{
		"email": email, "emailCode": emailCode, "nickname": nickname, "password": password,
	}
	if phone != "" {
		payload["phone"] = phone
	}
	_, err := forwardBuyerRegister("/api/v1/store/register", "/api/user-panel/register", payload)
	delete(payload, "password")
	if err != nil {
		storeFail(c, 400, plainRegisterMessage(err.Error(), password))
		return
	}
	code, err := finishBuyerBind(c, email, password, "user")
	if err != nil {
		hint := plainRegisterMessage(err.Error(), password)
		if code >= 500 {
			storeFail(c, 500, "账号已经注册，但本机没有记住绑定。请用这个邮箱再登录一次。")
			return
		}
		storeFail(c, 400, "账号已经注册，但绑定没有完成："+hint)
		return
	}
	writeFreshBuyerBind(c)
}

func registerEmailOK(email string) bool {
	if email == "" || strings.ContainsAny(email, " \t") || !strings.Contains(email, "@") {
		return false
	}
	parts := strings.Split(email, "@")
	return len(parts) == 2 && parts[0] != "" && strings.Contains(parts[1], ".")
}

func registerInputProblem(email, emailCode, nickname, password, phone string) string {
	if !registerEmailOK(email) {
		return "请输入有效的邮箱地址"
	}
	if len(emailCode) != 6 || strings.Trim(emailCode, "0123456789") != "" {
		return "请填写 6 位数字验证码"
	}
	if nickname == "" {
		return "请填写用户名"
	}
	if len([]rune(nickname)) > 50 {
		return "用户名不能超过 50 个字"
	}
	if len(password) < 6 {
		return "请设置密码，至少 6 位"
	}
	if phone != "" && !phoneRegexp.MatchString(phone) {
		return "手机号格式不正确"
	}
	return ""
}

func buyerStoreOrderBody(planID int64, itemKind, itemID, payMethod string) (map[string]any, error) {
	switch itemKind {
	case "plugin", "template":
		itemID = strings.TrimSpace(itemID)
		if itemID == "" {
			if itemKind == "template" {
				return nil, errors.New("请选择要购买的模板")
			}
			return nil, errors.New("请选择要购买的插件")
		}
		body := map[string]any{"itemKind": itemKind, "itemId": itemID}
		if strings.TrimSpace(payMethod) != "" {
			body["payMethod"] = strings.TrimSpace(payMethod)
		}
		return body, nil
	default:
		if planID <= 0 {
			return nil, errors.New("请选择商业版套餐")
		}
		body := map[string]any{"itemKind": "edition", "planId": planID}
		if strings.TrimSpace(payMethod) != "" {
			body["payMethod"] = strings.TrimSpace(payMethod)
		}
		return body, nil
	}
}

// BuyerStoreOrderCreate 向源站下商业版套餐或单品订单。
// itemKind 为 plugin 或 template 时必须有 itemId；否则必须有 planId。
// 源站明确吊销绑定时返回 400，并带 rebind，前端据此回到绑定步骤。其它失败也是 400。
func BuyerStoreOrderCreate(c *gin.Context) {
	var req struct {
		PlanID    int64  `json:"planId"`
		ItemKind  string `json:"itemKind"`
		ItemID    string `json:"itemId"`
		PayMethod string `json:"payMethod"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		storeFail(c, 400, "参数错误")
		return
	}
	body, err := buyerStoreOrderBody(req.PlanID, req.ItemKind, req.ItemID, req.PayMethod)
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
// 本地已经没有令牌时不再请求源站。源站说绑定已删除或令牌失效时，本地清掉后仍返回成功，
// 避免购买窗口因为删一条已经不存在的绑定而报错。源站暂时连不上也会清本地。
func BuyerStoreLogout(c *gin.Context) {
	if _, _, err := openBuyerBindingSecret(); err != nil {
		clearBuyerLocalBinding()
		storeData(c, gin.H{"ok": true})
		return
	}
	_ = signedSourceJSON(http.MethodPost, "/api/v1/store/auth/logout", map[string]any{}, nil)
	clearBuyerLocalBinding()
	storeData(c, gin.H{"ok": true})
}

// BuyerStoreManageLink 用已绑定的签名向源站要一条一次性登录链接。
// 链接只在响应里交给浏览器，本函数不写日志。绑定已失效时清本地并要求重新绑定。
func BuyerStoreManageLink(c *gin.Context) {
	var payload map[string]any
	err := signedSourceJSON(http.MethodPost, "/api/v1/store/auth/handoff", map[string]any{}, &payload)
	if err != nil {
		if storeFailSource(c, err) {
			return
		}
		storeFail(c, 400, plainManageLinkError(err.Error()))
		return
	}
	data, _ := payload["data"].(map[string]any)
	raw, _ := data["url"].(string)
	link, err := buyerManageLinkAllowed(raw)
	if err != nil {
		storeFail(c, 400, err.Error())
		return
	}
	storeData(c, gin.H{"url": link})
}

func plainManageLinkError(msg string) string {
	switch {
	case strings.Contains(msg, "无法连接源站"), strings.Contains(msg, "网络"):
		return "网络不通，请稍后再试"
	case strings.Contains(msg, "尚未绑定"):
		return "尚未绑定源站账号"
	case strings.Contains(msg, "无法解析"), strings.Contains(msg, "还没有这个接口"):
		return "源站暂时不能打开我的授权，请稍后再试"
	case strings.TrimSpace(msg) == "":
		return "暂时打不开我的授权，请稍后再试"
	default:
		return msg
	}
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
