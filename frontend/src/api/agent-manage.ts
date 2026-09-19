/**
 * 代理商管理相关 API
 *
 * 覆盖代理商、等级、开码配额、财务流水、升级审计五个页面的数据接口。
 * 注意：该模块下后端接口统一从 query 读取参数（包括 POST/PUT），
 * 因此提交数据时使用 params 而不是 data，请勿随意改动。
 */
import request from '@/utils/http'

// ==================== 类型定义 ====================

/** 通用分页响应（后端使用 list/total 字段） */
interface ListResponse<T> {
  list: T[]
  total: number
}

/** 代理商等级选项 */
export interface AgentLevelOption {
  code: string
  name: string
  discount: number
}

/** 代理商下拉选项 */
export interface AgentSelectOption {
  id: number | string
  name: string
}

/** 代理商列表项 */
export interface AgentItem {
  id: number
  name: string
  contact: string
  level: string
  levelLabel: string
  discount: number
  balance: number
  totalLicenses: number
  status: string
  statusLabel: string
  source: string
  sourceLabel: string
  originalUserId?: number
  transferredBalance?: number
  migratedLicenseCount?: number
  remark: string
  createdAt: string
}

/** 代理商列表搜索参数 */
export interface AgentSearchParams {
  page: number
  pageSize: number
  keyword?: string
  level?: string
  status?: string
  source?: string
}

/** 代理商等级列表项 */
export interface AgentLevelItem {
  id: number
  name: string
  discount: number
  selfServiceEnabled: boolean
  upgradePrice: number
  openingBonus: number
  benefits: string
  sort: number
  enabled: boolean
  remark: string
  agentCount: number
  createdAt?: string
  updatedAt: string
}

/** 代理商等级搜索参数 */
export interface AgentLevelSearchParams {
  page: number
  pageSize: number
  keyword?: string
  status?: string
}

/** 开码配额列表项 */
export interface QuotaItem {
  id: number
  agentId: number
  agentName: string
  appId: number
  appName: string
  totalQuota: number
  usedQuota: number
  price: number
  updatedAt: string
}

/** 开码配额搜索参数 */
export interface QuotaSearchParams {
  page: number
  pageSize: number
  agentId?: string | number
  appId?: string | number
}

/** 财务流水列表项 */
export interface TransactionItem {
  id: number
  orderNo: string
  agentName: string
  type: string
  typeLabel: string
  amount: number
  balanceAfter: number
  remark: string
  createdAt: string
}

/** 财务流水搜索参数 */
export interface TransactionSearchParams {
  page: number
  pageSize: number
  agentId?: string | number
  type?: string
  startDate?: string
  endDate?: string
}

/** 财务流水统计 */
export interface TransactionStats {
  totalRecharge: number
  totalConsume: number
  monthRecharge: number
  monthConsume: number
}

/** 升级审计统计 */
export interface AgentUpgradeStats {
  totalOrders: number
  pendingOrders: number
  completedOrders: number
  failedOrders: number
  completedAmount: number
  completedConversions: number
  transferredBalance: number
  openingBonus: number
  migratedLicenses: number
}

/** 升级订单列表项 */
export interface UpgradeOrderItem {
  id: number
  orderNo: string
  userId: number
  userEmail: string
  userName: string
  levelId: number
  levelCode: string
  levelName: string
  discount: number
  amount: number
  openingBonus: number
  paidAmount: number | null
  payChannel: string
  payMethod: string
  status: string
  agentId: number | null
  agentName: string
  gatewayTradeNo: string
  errorMessage: string
  createdAt: string
  paidAt: string
  completedAt: string
  updatedAt: string
}

/** 升级订单搜索参数 */
export interface UpgradeOrderSearchParams {
  page: number
  pageSize: number
  keyword?: string
  status?: string
  payChannel?: string
}

/** 账户转换记录列表项 */
export interface AccountConversionItem {
  id: number
  conversionNo: string
  orderNo: string
  userId: number
  userEmail: string
  userName: string
  agentId: number | null
  agentEmail: string
  agentName: string
  levelId: number
  levelName: string
  status: string
  openingFee: number
  transferredBalance: number
  openingBonus: number
  finalBalance: number
  migratedLicenseCount: number
  errorMessage: string
  startedAt: string
  completedAt: string
  createdAt: string
  updatedAt: string
}

/** 账户转换记录搜索参数 */
export interface ConversionSearchParams {
  page: number
  pageSize: number
  keyword?: string
  status?: string
}

/** 账户转换审计快照详情 */
export interface ConversionDetail {
  id: number
  conversionNo: string
  orderNo: string
  status: string
  errorMessage: string
  sourceSnapshot: unknown
  resultSnapshot: unknown
}

// ==================== 代理商 ====================

/** 代理商列表 */
export function fetchAgentList(params: AgentSearchParams) {
  return request.get<ListResponse<AgentItem>>({ url: '/api/agent/list', params })
}

/** 代理商下拉选项 */
export function fetchAgentSelectList() {
  return request.get<AgentSelectOption[]>({ url: '/api/agent/select-list' })
}

/** 新增代理商 */
export function fetchCreateAgent(params: {
  name: string
  contact: string
  password: string
  level: string
  discount: number
  remark: string
}) {
  return request.post({ url: '/api/agent/create', params })
}

/** 编辑代理商 */
export function fetchUpdateAgent(
  id: number,
  params: {
    name: string
    contact: string
    password: string
    level: string
    discount: number
    remark: string
  }
) {
  return request.put({ url: `/api/agent/${id}`, params })
}

/** 冻结/解冻代理商 */
export function fetchToggleAgent(id: number, status: string) {
  return request.put({ url: `/api/agent/${id}/toggle`, params: { status } })
}

/** 删除代理商 */
export function fetchDeleteAgent(id: number) {
  return request.del({ url: `/api/agent/${id}` })
}

/** 代理商充值 */
export function fetchRechargeAgent(id: number, params: { amount: number; remark: string }) {
  return request.post({ url: `/api/agent/${id}/recharge`, params })
}

/** 管理员代登录代理商 */
export function fetchImpersonateAgent(id: number) {
  return request.post<{
    accessToken: string
    agentId: number
    email: string
    name: string
    balance: number
  }>({ url: `/api/agent/${id}/impersonate` })
}

// ==================== 代理商等级 ====================

/** 将 select-list 响应规范成 ElSelect 可用的数组（已解包 data）。 */
export function unwrapAgentLevelOptions(data: unknown): AgentLevelOption[] {
  return Array.isArray(data) ? data : []
}

/** 等级下拉选项。GET /api/agent-level/select-list 的 data 必须是 {code,name,discount}[]。 */
export function fetchAgentLevelOptions() {
  return request
    .get<AgentLevelOption[]>({ url: '/api/agent-level/select-list' })
    .then(unwrapAgentLevelOptions)
}

/** 等级列表 */
export function fetchAgentLevelList(params: AgentLevelSearchParams) {
  return request.get<ListResponse<AgentLevelItem>>({ url: '/api/agent-level/list', params })
}

/** 等级提交参数 */
export interface AgentLevelPayload {
  name: string
  discount: number
  selfServiceEnabled: boolean
  upgradePrice: number
  openingBonus: number
  benefits: string
  sort: number
  enabled: boolean
  remark: string
}

/** 新增等级 */
export function fetchCreateAgentLevel(params: AgentLevelPayload) {
  return request.post({ url: '/api/agent-level/create', params })
}

/** 编辑等级（修改折扣会同步更新已绑定该等级的代理商） */
export function fetchUpdateAgentLevel(id: number, params: AgentLevelPayload) {
  return request.put({ url: `/api/agent-level/${id}`, params })
}

/** 删除等级 */
export function fetchDeleteAgentLevel(id: number) {
  return request.del({ url: `/api/agent-level/${id}` })
}

// ==================== 开码配额 ====================

/** 配额列表 */
export function fetchQuotaList(params: QuotaSearchParams) {
  return request.get<ListResponse<QuotaItem>>({ url: '/api/quota/list', params })
}

/** 分配配额 */
export function fetchCreateQuota(params: {
  agentId: number
  appId: number
  totalQuota: number
  price: number
}) {
  return request.post({ url: '/api/quota/create', params })
}

/** 调整配额 */
export function fetchUpdateQuota(id: number, params: { totalQuota: number; price: number }) {
  return request.put({ url: `/api/quota/${id}`, params })
}

/** 移除配额 */
export function fetchDeleteQuota(id: number) {
  return request.del({ url: `/api/quota/${id}` })
}

// ==================== 财务流水 ====================

/** 流水统计 */
export function fetchTransactionStats() {
  return request.get<TransactionStats>({ url: '/api/transaction/stats' })
}

/** 流水列表 */
export function fetchTransactionList(params: TransactionSearchParams) {
  return request.get<ListResponse<TransactionItem>>({ url: '/api/transaction/list', params })
}

// ==================== 升级审计 ====================

/** 升级审计统计 */
export function fetchAgentUpgradeStats() {
  return request.get<AgentUpgradeStats>({ url: '/api/admin/agent-upgrade/stats' })
}

/** 升级订单列表 */
export function fetchAgentUpgradeOrders(params: UpgradeOrderSearchParams) {
  return request.get<ListResponse<UpgradeOrderItem>>({
    url: '/api/admin/agent-upgrade/orders',
    params
  })
}

/** 账户转换记录列表 */
export function fetchAgentUpgradeConversions(params: ConversionSearchParams) {
  return request.get<ListResponse<AccountConversionItem>>({
    url: '/api/admin/agent-upgrade/conversions',
    params
  })
}

/** 账户转换审计快照 */
export function fetchAgentUpgradeConversionDetail(id: number) {
  return request.get<ConversionDetail>({ url: `/api/agent-upgrade/conversions/${id}` })
}
