import { AppRouteRecord } from '@/types/router'
import { dashboardRoutes } from './dashboard'
import { systemRoutes } from './system'
import { resultRoutes } from './result'
import { exceptionRoutes } from './exception'
import { licenseRoutes } from './license'
import { agentRoutes } from './agent'
import { piracyRoutes } from './piracy'
import { sdkRoutes } from './sdk'
import { sourceStationRoutes } from './source-station'
import { customerServiceRoutes } from './customer-service'

/**
 * 导出所有模块化路由
 * 顺序按运营工作流：高频办理 → 源站 → 风控客服 → 接入与系统
 */
export const routeModules: AppRouteRecord[] = [
  dashboardRoutes,
  licenseRoutes,
  agentRoutes,
  sourceStationRoutes,
  piracyRoutes,
  customerServiceRoutes,
  sdkRoutes,
  systemRoutes,
  resultRoutes,
  exceptionRoutes
]
