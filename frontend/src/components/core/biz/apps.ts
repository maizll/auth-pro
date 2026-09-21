export interface BizAppOption {
  id: number | string
  name: string
  appKey?: string
}

export type BizAppValueKey = 'id' | 'appKey'

export type BizAppApi =
  | 'license-options'
  | 'license-apps'
  | 'promotion'
  | 'user-panel'
  | 'agent-panel'

export type BizAppLoader = () => Promise<unknown>

const panelApps: Record<'user-panel' | 'agent-panel', { url: string; tokenKey: string }> = {
  'user-panel': { url: '/api/user-panel/apps', tokenKey: 'user_panel_token' },
  'agent-panel': { url: '/api/agent-panel/apps', tokenKey: 'agent_panel_token' }
}

export function appLoadErrorText(api: BizAppApi): string | undefined {
  if (api === 'user-panel') return '加载应用列表失败'
  return undefined
}

export function normalizeAppOptions(raw: unknown): BizAppOption[] {
  if (!Array.isArray(raw)) return []
  const options: BizAppOption[] = []
  for (const item of raw) {
    if (!item || typeof item !== 'object') continue
    const record = item as Record<string, unknown>
    const id = record.id
    const name = record.name
    if ((typeof id !== 'number' && typeof id !== 'string') || id === '') continue
    if (typeof name !== 'string' || !name.trim()) continue
    const option: BizAppOption = { id, name }
    if (typeof record.appKey === 'string' && record.appKey) option.appKey = record.appKey
    options.push(option)
  }
  return options
}

export function appOptionValue(
  app: BizAppOption,
  valueKey: BizAppValueKey
): string | number | undefined {
  if (valueKey === 'appKey') return app.appKey
  return app.id
}

export const panelAppPresets: Record<
  'user-panel' | 'agent-panel',
  { url: string; tokenKey: string }
> = panelApps
