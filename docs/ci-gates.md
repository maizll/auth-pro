# 发布关卡与 CI 策略（优化 #2）

本文说明 **auth-pro 三道发布关卡** 如何进 GitHub Actions，以及打 `vX.Y.Z` / 发 Release 时什么会拦住。客户端镜像仓 `maizll/auth-pro-client` 仍由同步得到同一套工作流；演练与跨版直升只在服务端发版前要求。

## 三道关卡（与 box 上 gate.sh 对齐）

| 关卡 | 内容 | 默认跑在哪里 |
| --- | --- | --- |
| **关卡 1 质量** | `quality-check.sh`、迁移调用环、`go test ./...`、MySQL/MariaDB handler 用例、宝塔脚本自检 `test-baota-scripts.sh` | PR + push `master`（CI）；打 tag 发版前再跑一遍（Release） |
| **关卡 2 构建** | 前端 `pnpm build`（含前端单测）、`build-release` 冒烟 / 正式签名打包 | PR：`frontend-build` + `build-smoke`；发版：正式私钥签名打包 |
| **关卡 3 演练** | `rehearsal.py`、跨版 `cross.py`、商业版 Playwright E2E（`CommercialMysqlE2E`） | **仅 box**（默认 runner 装不下完整宝塔/nginx/历史包）。发版前必须有 Commit Status 证明 |

不削弱任何断言：box 上因沙箱收养孤儿进程而放宽 PPID 的做法，**不得**写死进仓库脚本；只用环境变量临时放行（见下）。

## PR / push `master`：必过的 GitHub Checks

工作流：`.github/workflows/ci.yml`

| Job 名 | 做什么 |
| --- | --- |
| `vet, staticcheck, vue-tsc, eslint, unused exports, go test` | 关卡 1 主体 |
| `migration call cycles` | 迁移调用环 |
| `frontend unit tests and vite build` | `pnpm -C frontend build` |
| `baota install/upgrade script suite` | `scripts/test-baota-scripts.sh`（**不**设 PPID 放行） |
| `MySQL handler tests` | 起 MySQL 8，跑 handler 下 MySQL/MariaDB 用例；**禁止 SKIP**；跳过过重的 `CommercialMysqlE2E`（归 box 关卡 3） |
| `startup smoke` | 真实二进制官网/客户站启动冒烟 |
| `build-release smoke` | `scripts/ci/build-smoke.sh`：未签名打包 + 目录结构核对（不消耗正式私钥） |

请在仓库 **Settings → Rules → Rulesets**（或 Branch protection）把上述 job 设为 **Required**：

- `vet, staticcheck, vue-tsc, eslint, unused exports, go test`
- `migration call cycles`
- `frontend unit tests and vite build`
- `baota install/upgrade script suite`
- `MySQL handler tests`
- `startup smoke`
- `build-release smoke`

设好后，Checks 红就无法合进 `master`（管理员若开了 bypass 仍可强行合，但不推荐）。

## 打 tag / 发 Release：什么会拦住

工作流：`.github/workflows/release.yml`（`push` 匹配 `v*`）

发版任务在 **`gh release create` 之前**依次：

1. **box 关卡证明**：Commit Status `auth-pro/box-gate` 必须为 `success`（见下一节）。
2. 质量检查、迁移环、`go test ./...`。
3. MySQL handler 用例（同 PR，禁止 SKIP）。
4. 宝塔脚本自检、startup smoke。
5. 用机密 `AUTH_PRO_UPDATE_SIGNING_KEY` 正式打包并验签，再创建 Release。

任一失败 → **不会**创建 GitHub Release / 上传安装包。  
说明：GitHub 无法在「推 tag」那一瞬间用 Actions 结果拦下 tag 本身；本策略保证 **坏 tag 发不出包**。合 `master` 靠 Required Checks；发版靠 Release 工作流重跑自动化关卡 + box 证明。

### 仓库变量 / 机密（你需要确认的）

| 名称 | 类型 | 作用 |
| --- | --- | --- |
| `AUTH_PRO_UPDATE_SIGNING_KEY` | Actions **Secret**（已有） | 正式包签名；缺则发版失败 |
| `AUTH_PRO_REQUIRE_BOX_GATE` | Actions **Variable**（可选） | 默认视为 `1`（强制 box 证明）。仅应急设为 `0`/`false`/`off` 可跳过证明并打警告 |

客户端仓同步工作流后同样受签名机密约束；box 证明主要约束 **服务端** `maizll/auth-pro` 发版。

## box 上如何登记关卡证明

三道关卡在 box 全绿后（例如 `gate-1.9.x/gate.sh` 终检）：

```bash
cd /workspace/dev/auth-pro
# 摘要与 gateN.status 最后几行对齐即可，最长 140 字
./scripts/ci/record-box-gate.sh \
  "QC0 CYC0 GOTEST0 MYSQL 7/0/1 BAOTA0 BUILD0 BC0 REH 96/96 CROSS 278/278"
```

失败时：

```bash
./scripts/ci/record-box-gate.sh --fail "CROSS 某站超时"
```

要求：本机 `gh` 已登录且对 `maizll/auth-pro` 有写 Commit Status 的权限。  
发版工作流用 `scripts/ci/verify-box-gate.sh` 读取该 Status；没有或不是 `success` 直接失败。

### 宝塔脚本 PPID（环境检测，不是永久豁免）

`scripts/test-baota-scripts.sh` 默认只接受孤儿 **PPID=1**（真实 Linux / GitHub runner）。

box 沙箱若收养到 `sand-exit-watch`（常见 52/53），**仅在 box 跑关时**设置：

```bash
export AUTH_PRO_BAOTA_ALLOW_ORPHAN_PPIDS=52,53
./scripts/test-baota-scripts.sh
```

不要把 `52|53` sed 进仓库脚本，也不要在 CI workflow 里设置该变量。

## 发版推荐顺序（服务端）

1. 功能分支 PR → CI 全绿 → squash 合进 `master`。
2. 在合并提交上跑 box 三道关卡；全过则 `record-box-gate.sh`。
3. `git tag vX.Y.Z <合并SHA> && git push origin vX.Y.Z`。
4. 等 Release 工作流：再跑自动化关卡 + 校验 box 证明 → 签名打包 → `gh release create`。
5. 再走客户端 sync / paid 发布与 `verify_release.sh`（既有流程不变）。

## 刻意不放进默认 runner 的部分

- 完整 `rehearsal.py` / `cross.py`（依赖本机 nginx、supervisor、历史发布包、演练密钥与域名）。
- `TestCommercialMysqlE2E`（Playwright + 双站 + 本机 hosts）。

这些仍是发版硬条件，只是证明方式改为 **box 跑完 + Commit Status**，而不是在 `ubuntu-latest` 上硬撑。

## 本改动范围

- 仅 CI / 文档 / baota PPID 环境检测；**不**发 1.9.2 产品功能，**不**改 `VERSION`（除非日后为发布工具链单独约定）。
- 客户端仓：下次从服务端同步工作流即可；无需为本优化单独发客户端版本。
