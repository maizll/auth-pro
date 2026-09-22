export interface HomeTemplateAction {
  label?: string
  type?: 'login'
}

export interface HomeTemplateFeature {
  icon?: string
  title: string
  description?: string
}

export type HomeTemplateStylePreset = 'cartoon-blue' | 'fintech-gold'

export interface HomeTemplateDocument {
  schemaVersion: 1
  stylePreset?: HomeTemplateStylePreset
  theme?: {
    primaryColor?: string
    backgroundColor?: string
    textColor?: string
  }
  hero: {
    badge?: string
    title: string
    highlight?: string
    description?: string
    imageUrl?: string
    primaryAction?: HomeTemplateAction
    secondaryAction?: HomeTemplateAction
  }
  features?: HomeTemplateFeature[]
  footer?: {
    text?: string
  }
}

export interface ActiveHomeTemplateResponse {
  id: number | 'default'
  templateId: string
  name: string
  version: string
  isDefault: boolean
  format?: 'json' | 'static'
  entryUrl?: string
  assetBaseUrl?: string
  schemaVersion?: number
  document?: HomeTemplateDocument
}

export function isHomeTemplateDocument(value: unknown): value is HomeTemplateDocument {
  if (!value || typeof value !== 'object') return false
  const document = value as Partial<HomeTemplateDocument>
  return (
    document.schemaVersion === 1 &&
    (!document.stylePreset || isHomeTemplateStylePreset(document.stylePreset)) &&
    Boolean(document.hero) &&
    typeof document.hero?.title === 'string' &&
    document.hero.title.trim().length > 0
  )
}

export function isHomeTemplateStylePreset(value: unknown): value is HomeTemplateStylePreset {
  return value === 'cartoon-blue' || value === 'fintech-gold'
}

export function resolveHomeTemplatePreset(value: unknown): HomeTemplateStylePreset | 'standard' {
  return isHomeTemplateStylePreset(value) ? value : 'standard'
}

/** 首页与宿主登录框共用的主题变量。 */
export function homeTemplateThemeStyle(
  document: Pick<HomeTemplateDocument, 'stylePreset' | 'theme'>
): Record<string, string> {
  const preset = resolveHomeTemplatePreset(document.stylePreset)
  const defaults =
    preset === 'fintech-gold'
      ? { primary: '#f0b90b', background: '#0b0e11', text: '#f5f5f5' }
      : { primary: '#4d6bfe', background: '#f7f8fc', text: '#172033' }
  const primary = safeTemplateColor(document.theme?.primaryColor, defaults.primary)
  const background = safeTemplateColor(document.theme?.backgroundColor, defaults.background)
  const text = safeTemplateColor(document.theme?.textColor, defaults.text)
  return {
    '--remote-primary': primary,
    '--remote-background': background,
    '--remote-text': text,
    '--gold': primary,
    '--page-bg': background,
    '--text': text
  }
}

export function safeTemplateColor(value: string | undefined, fallback: string): string {
  return value && /^#[0-9a-f]{3,8}$/i.test(value) ? value : fallback
}

export function safeTemplateImageURL(value: string | undefined): string {
  if (!value) return ''
  try {
    const parsed = new URL(value, window.location.origin)
    return ['http:', 'https:'].includes(parsed.protocol) ? parsed.toString() : ''
  } catch {
    return ''
  }
}

export function safeTemplateIcon(value: string | undefined): string {
  return value && /^ri:[a-z0-9-]+$/i.test(value) ? value : 'ri:sparkling-line'
}

export function isStaticHomeTemplateEntryURL(value: unknown): value is string {
  return (
    typeof value === 'string' &&
    /^\/api\/home-template\/assets\/[1-9]\d*\/upload-\d+\/index\.html$/.test(value)
  )
}

export function resolveHomeTemplateAssets(
  document: HomeTemplateDocument,
  assetBaseUrl?: string
): HomeTemplateDocument {
  const image = document.hero.imageUrl
  if (
    !image ||
    !assetBaseUrl ||
    !/^\/api\/home-template\/assets\/[1-9]\d*\/upload-\d+\/$/.test(assetBaseUrl) ||
    /^(?:[a-z][a-z0-9+.-]*:|\/\/)/i.test(image)
  )
    return document
  const base = new URL(assetBaseUrl, window.location.origin).href
  const resolved = new URL(image.replace(/^\/+/, ''), base).href
  return {
    ...document,
    hero: { ...document.hero, imageUrl: resolved.startsWith(base) ? resolved : '' }
  }
}
