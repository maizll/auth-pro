import axios from 'axios'
import { fetchLicenseAppList, fetchLicenseAppOptions } from '@/api/license-manage'
import { fetchPromotionApps } from '@/api/promotion-campaign'
import { normalizeAppOptions, panelAppPresets, type BizAppApi, type BizAppOption } from './apps'

async function loadPanelApps(api: 'user-panel' | 'agent-panel'): Promise<BizAppOption[]> {
  const preset = panelAppPresets[api]
  const token = localStorage.getItem(preset.tokenKey) || ''
  const { data } = await axios.get(preset.url, {
    headers: { Authorization: `Bearer ${token}` }
  })
  if (data?.code !== 200) return []
  return normalizeAppOptions(data.data)
}

/** 复用现有应用列表接口，不新增后端路由。 */
export async function loadBizApps(api: BizAppApi): Promise<BizAppOption[]> {
  if (api === 'license-options') return normalizeAppOptions(await fetchLicenseAppOptions())
  if (api === 'license-apps') return normalizeAppOptions(await fetchLicenseAppList())
  if (api === 'promotion') return normalizeAppOptions(await fetchPromotionApps())
  return loadPanelApps(api)
}
