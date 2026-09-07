/**
 * 工单系统 API（管理端）
 *
 * 面板端（用户/代理商）走 localStorage token，直接使用 axios，
 * 相关请求逻辑封装在 components/core/panels 的工单组件内部；
 * 本模块只承载管理端请求与三端共享的类型定义。
 */
import request from '@/utils/http'

// ==================== 类型定义 ====================

export type TicketStatus = 'pending' | 'replied' | 'closed'
export type TicketSenderType = 'user' | 'agent' | 'admin'

export interface TicketItem {
  id: number
  ticketNo: string
  creatorType: 'user' | 'agent'
  creatorId: number
  creatorName: string
  category: string
  categoryText: string
  title: string
  priority: string
  status: TicketStatus
  statusText: string
  lastReplyAt: string
  lastReplyBy: string
  closedAt: string | null
  createdAt: string
  unread: boolean
}

export interface TicketMessage {
  id: number
  ticketId: number
  senderType: TicketSenderType
  senderId: number
  senderName: string
  content: string
  createdAt: string
}

export interface TicketDetail {
  ticket: TicketItem
  messages: TicketMessage[]
}

export interface TicketListParams {
  page: number
  pageSize: number
  status?: TicketStatus | ''
  category?: string
  creatorType?: 'user' | 'agent' | ''
  keyword?: string
}

export interface TicketStats {
  pending: number
  today: number
  closed: number
}

/** 工单分类选项（与后端白名单一致） */
export const TICKET_CATEGORY_OPTIONS = [
  { value: 'authorization', label: '授权问题' },
  { value: 'payment', label: '支付问题' },
  { value: 'deploy', label: '部署咨询' },
  { value: 'other', label: '其他' }
] as const

/**
 * 工单列表响应（list/total + 统计）
 * 继承 PaginatedResponse 是为了让 useTable 推导出记录类型，
 * 实际后端返回的是 list/total/stats 字段。
 */
export interface TicketListResponse extends Api.Common.PaginatedResponse<TicketItem> {
  list: TicketItem[]
  stats: TicketStats
}

// ==================== 管理端接口 ====================

/** 工单列表 + 统计 */
export function fetchTicketList(params: TicketListParams) {
  return request.get<TicketListResponse>({
    url: '/api/ticket/list',
    params
  })
}

/** 工单详情（读取后管理端已读游标推进） */
export function fetchTicketDetail(id: number) {
  return request.get<TicketDetail>({ url: `/api/ticket/${id}` })
}

/** 管理员回复工单 */
export function fetchTicketReply(id: number, content: string) {
  return request.post<{ messageId: number }>({ url: `/api/ticket/${id}/reply`, data: { content } })
}

/** 关闭 / 重开工单 */
export function fetchTicketStatus(id: number, action: 'close' | 'reopen') {
  return request.put({ url: `/api/ticket/${id}/status`, data: { action } })
}

/** 管理端未读工单数 */
export function fetchTicketUnreadCount() {
  return request.get<{ count: number }>({ url: '/api/ticket/unread-count' })
}
