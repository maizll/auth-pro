# auth-pro 业务逻辑安全审计

审计方式：只读核对当前仓库中的中间件、路由、handler、安装流程与余额扣款实现。不包含修复。只记录能从现有代码路径走到的问题。

范围：

1. 权限越权（代登录、`RequireSuperAdmin` / `RequireAdmin`）
2. JWT 会话（refresh / access 隔离、停用与角色变更后的失效、令牌篡改）
3. 余额并发（购买事务、`FOR UPDATE`、超卖、负数余额）
4. 安装锁（重复安装、删表、新建管理员）
5. 审计日志（代登录、改角色、扣款，以及日志能否被业务接口改删）
6. 边界（401/403、异常参数、空值、非法 ID）

近期提交 `99eb594`、`95d7c57` 已收紧「同一已有数据库上的重装」和「管理员 refresh 令牌、停用、代登录入口」。下文的安装与会话问题是这些修复没有盖住的路径。

---

【高｜代理商冻结后旧会话仍可改授权】

漏洞描述

`AgentToggle`（`backend/handler/agent.go`）只把 `agents.enabled` 写成 0，不更新 `password_changed_at`。代理端路由在 `backend/main.go` 的 `agentSecured` 上只挂了 `JWTAuth` 和 `RequireFreshPassword("agents")`，没有等价于用户端 `RequireActiveUser` 的启用复查。`getAgentID` / `requireAgentRole`（`backend/handler/agent_panel.go`）只看令牌里的 `role == "agent"`。

因此冻结只挡住那些 SQL 自带 `enabled = 1` 的入口（例如 `AgentPanelPurchase`、`AgentPanelBalance`、`currentAgentDiscount`，以及卡密兑换里对主体行的 `FOR UPDATE`）。下面这些写操作不查 `enabled`，冻结后 7 天访问令牌到期前仍然成功：

- `AgentPanelLicenseUpdate`：改授权绑定目标
- `AgentPanelLicenseRefreshKey`：轮换密钥授权
- `AgentLicenseSiteUnbind`（`backend/handler/license_site_manage.go`）：解绑站点
- `AgentPanelUpdateProfile`、`AgentPanelChangePassword`（`backend/handler/agent_panel_profile.go`）
- `PanelTicketCreate` 等工单接口（只判断 `role` 为 `user` 或 `agent`）

用户端没有同类问题：`userSecured` 使用 `RequireActiveUser`，禁用或已升级账号会在每次请求被拒绝。管理员端由 `RequireAdmin` 复查 `admins.enabled`。

复现步骤

身份：已登录且尚未过期的代理商；另一名管理员。

条件：管理员调用 `AgentToggle`，请求状态为冻结（`enabled` 置 0），且不同时修改该代理商密码。代理商继续用冻结前由 `AgentPanelLogin` 签发的访问令牌，调用 `PUT /api/agent-panel/licenses/:id` 或 `POST /api/agent-panel/licenses/:id/refresh-key`。`getAgentID` 因令牌角色仍是 `agent` 而通过，更新语句只按 `owner_type = 'agent' AND owner_id = ?` 匹配，不读 `agents.enabled`。

风险影响

冻结不能立刻收回授权绑定、密钥和站点解绑能力。令牌默认 7 天（`AgentPanelLogin`）。被冻结的代理，或持有其令牌的人，仍可改写其名下授权，直到令牌过期或密码被修改（后者才会刷新 `password_changed_at`）。

修复建议

在代理端中间件按 `agents.enabled` 复查，冻结、删除账号时拒绝旧令牌。冻结时同时写入 `password_changed_at`（或独立的会话版本），使已签发令牌立刻失效。购买、余额、卡密路径上已有的 `enabled = 1` 条件保留，但不能代替入口复查。

---

【高｜安装锁丢失后，初始化接口可把进程改接到空库并新建超级管理员】

漏洞描述

`InstallGuard`（`backend/middleware/install_guard.go`）只根据 `install.lock` 是否存在决定是否关闭安装写接口。锁不存在时，无 `Origin` 或与请求 Host 同源的调用可以进入 `InstallInitTables` 和 `InstallCreateAdmin`（`backend/handler/install.go`）。

`installDatabaseHasData` 只统计「当前这条连接」上 `admins`、`users`、`agents`、`apps`、`licenses`、`license_plans`、`transactions`、`license_purchase_orders` 是否有行。它不读取本机已经在用的 `db.json`，也不比较请求体里的库是不是正在服务的库。

`InstallInitTables` 在判定该连接为空后执行嵌入的 `schema.sql`（含 `DROP TABLE IF EXISTS`），然后调用 `config.SaveDBConfig`。`SaveDBConfig` 同时覆盖磁盘 `db.json` 和进程内 `dbConfig`。`config.DB`（`backend/config/db.go`）在 DSN 变化时重建连接池，旧池约 30 秒后关闭。随后 `InstallCreateAdmin` 通过 `LoadDBConfig` 读到新配置；新库仍为空，于是插入 `role_id = 1` 的管理员并 `CreateLockFile`。

同一生产库上「已有业务行则拒绝 DROP / 拒绝再建管理员」仍然有效。绕过点是：检查发生在攻击者指定的空库上，通过之后生产配置被换成这套空库。

例外：若进程设置了 `AUTO_PRO_DB_HOST`，`LoadDBConfig` 优先返回环境变量，内存缓存被换掉也不会让正在运行的 `DB()` 改走新库。此时 `db.json` 仍会被写成新连接，只在以后改为读文件配置时生效。默认安装向导走 `db.json`，不依赖该环境变量。

复现步骤

身份：未登录。前提是数据目录中的 `install.lock` 已不存在（删除、漏拷贝或只恢复了数据库），且运行中的数据库配置来自 `db.json` 而不是 `AUTO_PRO_DB_HOST`。

条件：调用 `POST /api/install/init-tables`，请求体中的数据库账号能够连上一个相对于 `installDataTables` 为空的 MySQL（表不存在或这些表均为 0 行），且该连接与当前 `db.json` 不是同一套已有数据的库。`InstallInitTables` 走空库分支并 `SaveDBConfig`。紧接着调用 `POST /api/install/create-admin`。`InstallCreateAdmin` 不再使用请求体里的数据库字段，而是 `LoadDBConfig()`；此时读到的是刚刚写入的空库，`installDatabaseHasData` 返回假，插入超级管理员并写回安装锁。

风险影响

原库数据不会被这条路径 `DROP`，但进程会改连空库，调用者成为该空库上的超级管理员，安装锁重新关上。原管理员无法再使用安装接口，在线服务也不再指向原库。持有原库口令并不是必要条件。

修复建议

锁文件缺失时，若本机 `db.json`（或环境变量 DSN）已经能连上含业务行的库，拒绝 `init-tables` 和 `create-admin`。禁止安装接口覆盖已有数据库配置。空库判定与建表、建管理员放在同一事务或安装互斥锁里，并且只允许对「当前配置所指向的库」执行。`schema.sql` 不应在已有库上整库 `DROP TABLE`。

---

【高｜管理员保存用户时用绝对余额覆盖并发扣款，且服务端允许负数】

漏洞描述

用户、代理的余额购买已改为事务内 `deductPurchaseBalance`（`backend/handler/balance_deduct.go`）：`SELECT balance ... FOR UPDATE`，再 `UPDATE ... SET balance = balance - ? WHERE id = ? AND balance >= ?`，`RowsAffected != 1` 则失败回滚，不会把事务外读到的余额写回去。

管理端更新用户没有走这条路径。`AdminUserUpdate`（`backend/handler/user_manage.go`）在请求体带 `balance` 时执行 `UPDATE users SET balance = ?`，无事务、无行锁、不要求 `balance >= 0`，也不写 `transactions` 或 `operation_logs`。`AdminUserCreate` 同样把指针里的余额原样插入。

前端编辑用户对话框（`frontend/src/views/system/user/modules/user-dialog.vue`）提交时总是带上打开对话框时的 `balance`。界面控件有 `min=0`，接口本身没有。表结构 `users.balance` 为 `DECIMAL(12,2)`，没有非负约束。

复现步骤

身份：任意能通过 `RequireAdmin` 的管理员（`PUT /api/user/:id` 挂在 `secured` 上，不是 `RequireSuperAdmin`）；同时有该用户自己的登录会话。

条件一（并发覆盖）：管理员打开用户编辑并停留在提交前。该用户用余额完成一次 `UserPurchase`，`deductPurchaseBalance` 提交后余额已减少且授权已插入。管理员再提交编辑。请求体里的 `balance` 仍是打开对话框时的旧值。`AdminUserUpdate` 的绝对赋值若在购买事务提交之后执行，会把已扣余额写回旧值。购买事务持有行锁时，这条 `UPDATE` 会等到锁释放后再写绝对值，结果相同。

条件二（负数）：同一接口的 `balance` 字段为负数（界面限制不作用于直接调用接口）。`req.Balance != nil` 分支不比较大小，直接写入。创建用户接口在同样条件下插入负余额。

风险影响

用户可以在扣款发证之后，因一次普通的管理员保存把余额恢复，形成「授权已发出、余额未净减少」。余额也可以被写成负数；之后购买路径的 `balance >= 金额` 会拒绝继续扣款，但账上已经为负，且没有流水可核对是谁改的。

修复建议

管理端不要回写绝对余额。调整余额应走独立的增减接口，使用 `balance = balance + ?` 或与 `deductPurchaseBalance` 相同的行锁和条件更新，并写入 `transactions`（操作者、变动额、变动后余额）。拒绝小于 0 的结果。用户编辑接口忽略 `balance` 字段，避免保存昵称时带上过期余额。

---

【中｜代登录令牌不随操作者停用失效，后续操作也不记真实管理员】

漏洞描述

代登录入口本身已限制为超级管理员：`POST /api/user/:id/impersonate` 与 `POST /api/agent/:id/impersonate` 在 `superSecured` 上，并再经 `rejectUnlessSuperAdmin`。`signImpersonateToken`（`backend/handler/impersonate.go`）写入 `act=impersonation` 和 `operator_id`，有效期 2 小时，且不改目标账号的 `last_login_*`。成功时 `writeImpersonationLog` 写入 `operation_logs`。普通管理员不能签发这种令牌。

签发之后，该令牌是用户或代理的访问令牌。`RequireAdmin` 对操作者账号的停用、角色编码变更，只作用于管理员自己的后台令牌。用户端 `RequireActiveUser`、代理端 `RequireFreshPassword` 都不读取 `operator_id`，也不检查操作者是否仍为启用的 `R_SUPER`。

`ImpersonationOperatorID`（`backend/middleware/jwt.go`）的注释写明供后续审计读取，生产 handler 中没有任何调用。代登录期间的购买、改授权、改密只记被代登录的用户或代理。

复现步骤

身份：超级管理员，以及事后将其停用或改掉 `role_id` / 角色编码的另一名超级管理员。

条件：第一名超级管理员调用代登录接口并取得 `accessToken`（`writeImpersonationLog` 已成功，否则接口不会返回令牌）。在该令牌 2 小时到期前，第二名超级管理员将其管理员账号 `enabled` 置 0，或将其 `role_code` 改成与令牌不一致。原管理员自己的后台令牌会被 `RequireAdmin` 拒绝。已发出的代登录令牌仍可访问对应用户端或代理端；代理目标若随后被冻结，还会叠加上一节「冻结不失效」的写接口。这些后续请求不会把 `operator_id` 写入 `transactions` 或 `operation_logs`。

风险影响

停用或降权超级管理员不能收回已经发出的代登录会话，最长 2 小时。此期间对用户或代理数据的修改，在业务流水中看起来像本人操作。`operation_logs` 里只有一次「开始代登录」，没有之后做了什么。

修复建议

代登录令牌每次使用时复查操作者仍启用且 `role_code` 仍为 `R_SUPER`，否则 401。业务写操作在检测到 `act=impersonation` 时把 `operator_id` 写入 `operation_logs` 或流水备注。停用管理员时使该操作者签发的代登录令牌失效（会话版本或服务端登记）。

---

【中｜首次安装创建管理员无互斥，并发可插入多名超级管理员】

漏洞描述

`InstallCreateAdmin` 先 `installDatabaseHasData`，再 `INSERT` 角色和管理员，最后 `CreateLockFile`。检查和插入不在同一事务里，也没有「尚无管理员才允许插入」的行锁或唯一的安装占位。`admins.username` 只保证用户名不重复，不保证全表只有一行。`InstallGuard` 在请求进入时看锁文件；两个请求都在锁文件创建前进入时，中间件不会拦截第二个。

复现步骤

身份：未登录。前提：尚未安装（无 `install.lock`，且 `installDataTables` 均为空）。这是安装窗口，不是锁丢失后的换库问题。

条件：两笔 `POST /api/install/create-admin` 使用不同的 `adminUsername`，并且都在对方执行 `INSERT INTO admins` 之前通过 `installDatabaseHasData`。两笔都会插入 `role_id = 1`，并各自尝试写锁文件。后写入的锁文件只表示「已安装」，不会删掉多出来的管理员。

风险影响

安装完成时除了操作者预期的账号外，还可以存在另一名超级管理员。安装接口在锁文件写上之后关闭，这名额外账号不会出现在安装向导的成功提示里。

修复建议

用文件锁或数据库事务包住「统计管理员 + 插入 + 写锁」。插入使用「仅当管理员数为 0」的条件，`RowsAffected` 不为 1 则回滚。第二名并发调用者应得到 403，且 `admins` 行数保持为 1。

---

【低｜改角色与改菜单无操作日志；授权校验日志可被普通管理员清空】

漏洞描述

代登录成功路径会写 `operation_logs`。余额购买和后台手工充值会写 `transactions`（`AgentRecharge` / `rechargeAgentManually` 的备注里带有管理员 ID）。全仓库没有 `UPDATE` 或 `DELETE` `operation_logs` 的业务接口，业务接口不能改删这张表。

以下敏感变更没有对应的 `operation_logs` 行：

- `RoleUpdate`、`RoleUpdateMenus`、`RoleDelete`（`backend/handler/role.go`）：改角色资料、整体替换 `role_menus`、删除角色
- `AdminUserUpdate` / `AdminUserCreate` 对 `users.balance` 的写入：不进 `operation_logs`，也不进 `transactions`

`VerifyLogClear`（`backend/handler/verify_log.go`）对 `verify_logs` 执行 `TRUNCATE`。路由在 `backend/main.go` 的 `secured.DELETE("/verify-log/clear")`，只要 `RequireAdmin` 通过即可，不要求 `R_SUPER`，清空前也不写 `operation_logs`。这是授权校验日志，不是 `operation_logs`，但属于可被业务接口整表删除的审计数据。

复现步骤

身份：超级管理员调用角色接口；任意管理员调用校验日志清空。

条件：`PUT /api/role/:id` 或 `PUT /api/role/:id/menus` 成功返回后，`operation_logs` 无新行。任意管理员调用 `DELETE /api/verify-log/clear` 时，`VerifyLogClear` 执行 `TRUNCATE TABLE verify_logs`，不记录操作者。

风险影响

角色菜单被整体替换、角色被关掉或删除后，无法从操作日志回答是谁在何时改的。授权校验记录可以被非超级管理员一次清空，且留不下「谁清空的」。`operation_logs` 里已有的代登录记录不能通过现有业务接口删掉。

修复建议

角色变更、菜单替换、管理员调整用户余额都写入 `operation_logs`（操作者、目标、前后差异）。清空校验日志改为超级管理员、按条件删除或归档，并先记一条操作日志。保持 `operation_logs` 对业务接口只插入。

---

## 已审计无发现问题

### 1. 普通管理员代登录与 RequireSuperAdmin / RequireAdmin

该项未发现「普通管理员提升为超级管理员，或横向代登录其他账号」的可复现路径。

- 两个代登录路由注册在 `superSecured`，中间件为 `RequireSuperAdmin`；`AdminImpersonateUser` / `AdminImpersonateAgent` 内还有 `rejectUnlessSuperAdmin`。
- 代登录令牌的 `role` 仅为 `user` 或 `agent`，不能通过 `RequireAdmin` 进入后台。
- `RequireAdmin` 每次用 `admins.id` 复查 `enabled`，并把库中的 `role_code` 与令牌比较，不一致则 401。`RequireSuperAdmin` 在此之后要求令牌中的 `role_code` 为 `R_SUPER`。停用管理员或改掉其角色编码后，后台旧令牌不能继续调用管理接口，也不能新签发代登录令牌。
- 仓库中不存在把 `admins.role_id` 改成超级管理员的业务接口。`UpdateUserInfo` 只更新昵称、邮箱、头像。
- 代登录签发失败（含写 `operation_logs` 失败）时不把令牌返回给调用方。

未纳入本项的问题：代理商冻结后的用户/代理令牌（见「高｜代理商冻结」），以及已经发出的代登录令牌不随操作者停用失效（见对应「中」项）。这不是普通管理员绕过 `RequireSuperAdmin`。

### 2. 管理员 refresh / access 隔离、管理员停用与令牌篡改

管理员登录路径未发现可复现问题。

- `Login` 为 `refreshToken` 设置 `typ=refresh`。`JWTAuth` 遇到该类型直接 401，不能当作接口 Bearer。仓库没有换发接口，因此这枚 7 天令牌不能用来延长后台会话。
- 访问令牌 24 小时，且带 `IssuedAt`。`RequireFreshPassword("admins")` 在 `password_changed_at` 晚于签发时间时拒绝。
- `JWTAuth` 只接受 HMAC 签名并使用安装时生成的密钥。非 HMAC 算法在解析回调中被拒绝，不能靠改算法字段伪造令牌。没有密钥则不能改 `role`、`role_code` 或去掉 `typ`。
- 用户禁用、用户升级为代理，由 `RequireActiveUser` 在每次用户端请求复查。

本项已单独成节的问题：代理商 `enabled=0` 不使代理端写接口失效；代登录令牌不复查操作者状态。

### 3. 余额购买事务与 FOR UPDATE

购买扣款路径未发现超卖、负余额或并发少扣。

- `UserPurchase` 与 `AgentPanelPurchase` 在事务内调用 `deductPurchaseBalance`，相对扣减且要求 `balance >= 金额`，影响行数必须为 1，否则回滚，不插入授权。
- 代理配额扣减为单条 `UPDATE agent_quotas SET used = used + 1 WHERE used < total`，第二笔并发在条件不满足时影响 0 行。
- `enforcePurchaseLimit` 对启用了限购的活动计划 `SELECT ... FOR UPDATE` 后再计数。线上单的 `insertAllowedLicensePurchaseOrder` 在同一事务里做该检查再插入 pending 订单。`settleLicensePurchaseOrder` 对订单行 `FOR UPDATE`，并核对实付金额与订单金额。
- 活动价计算把结果限制在不小于 0。负数扣款金额在 `deductPurchaseBalance` 入口被拒绝。
- 充值入账 `increaseSubjectBalance` 使用 `balance = balance + ?`。账户升级对用户行 `FOR UPDATE` 后再把余额置 0。

本项已单独成节的问题：`AdminUserUpdate` 的绝对余额赋值可以在购买提交之后把余额写回去，并接受负数。那不是购买事务内部的丢更新。

### 4. 安装锁：同一已有库上的重放

在「请求所连接的库就是已有数据的那个库」这一前提下，未发现删表或再建管理员的绕过。

- 存在 `install.lock` 时，`InstallGuard` 对 `init-tables` 和 `create-admin` 返回 403。`InstallStatus` 只读，不写库。
- 锁丢失但该连接上任一 `installDataTables` 已有行时，`InstallInitTables` 不执行 `schema.sql`，`InstallCreateAdmin` 不插入管理员、不写锁。探测语句失败时同样拒绝，不会把错误当成空库。
- `InstallCreateAdmin` 使用已保存的配置，不使用请求体里的数据库口令去连生产库。因此仅调用 `create-admin`、而生产库已有数据时，不能新建超级管理员。

本项已单独成节的问题：改连另一个空库从而绕过上述计数；安装窗口内并发 `create-admin`。

### 5. 已有操作日志不能被业务接口改删

`operation_logs` 的插入点（代登录、卡密兑换、授权站点变更）没有对应的更新或删除接口。购买扣款和代理商手工充值记在 `transactions`，同样没有业务删除接口。代登录入口日志在返回令牌之前写入，写入失败则调用方拿不到令牌。

本项已单独成节的问题：改角色、改菜单、管理员改用户余额没有日志；`verify_logs` 可被清空；代登录之后的业务动作不记录 `operator_id`。

### 6. 401/403、空值与非法 ID

该项未发现可复现的鉴权绕过。

- 缺少或格式错误的 `Authorization`、签名无效、过期、`typ=refresh`，由 `JWTAuth` 以 HTTP 401 中止，后续 handler 不执行。
- `RequireAdmin` / `RequireSuperAdmin` 在角色不符时写入业务码 403 并 `Abort`。HTTP 状态是 200、业务码是 403，这是现有接口习惯，处理器不会继续执行，因此不能把「HTTP 200」当成已授权。
- 代登录的路径 ID 无法解析或为 0 时，`AdminImpersonateUser` / `AdminImpersonateAgent` 返回 400，不签发令牌。目标不存在或已禁用时不签发。
- 用户购买、代理购买对授权类型、空目标、非正 ID 在进入扣款前返回 400。SQL 条件使用占位符，非法 ID 不会改变 WHERE 逻辑。
- 管理端部分更新（如 `AdminUserUpdate`）在 ID 不存在时仍可能返回「更新成功」，因为未检查影响行数。语句是参数化的，影响行数为 0，不产生越权或余额变化。不单列为空漏洞。
