import { AppRouteRecord } from '@/types/router'
import { dashboardRoutes } from './dashboard'
import { systemRoutes } from './system'
import { resultRoutes } from './result'
import { exceptionRoutes } from './exception'
import { licenseRoutes } from './license'
import { agentRoutes } from './agent'
import { piracyRoutes } from './piracy'
import { sdkRoutes } from './sdk'
import { pluginStoreRoutes } from './plugin-store'
import { onlineUpdateRoutes } from './online-update'
import { sourceStationRoutes } from './source-station'
import { customerServiceRoutes } from './customer-service'

/**
 * 导出模块化路由（组件注册）。
 * 产品侧栏的顺序/标题/显隐以后端菜单为准（VITE_ACCESS_MODE=backend）。
 * 本数组顺序仅在 frontend 演示模式下影响侧栏。
 */
export const routeModules: AppRouteRecord[] = [
  dashboardRoutes,
  licenseRoutes,
  agentRoutes,
  sourceStationRoutes,
  piracyRoutes,
  customerServiceRoutes,
  sdkRoutes,
  pluginStoreRoutes,
  onlineUpdateRoutes,
  systemRoutes,
  resultRoutes,
  exceptionRoutes
]
