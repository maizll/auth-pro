export const CATALOG_ID_PATTERN = /^[a-z0-9][a-z0-9-]{1,58}$/
export const SHA256_HEX_PATTERN = /^[a-fA-F0-9]{64}$/

const FALLBACK_PREFIX = {
  plugin: 'plugin',
  template: 'template'
} as const

export type CatalogKind = keyof typeof FALLBACK_PREFIX

/**
 * Frequent characters in plugin/template display names.
 * Unlisted CJK is dropped so Chinese-only names fall back to plugin-/template- slugs.
 */
const PINYIN: Record<string, string> = {
  一: 'yi',
  二: 'er',
  三: 'san',
  四: 'si',
  五: 'wu',
  六: 'liu',
  七: 'qi',
  八: 'ba',
  九: 'jiu',
  十: 'shi',
  百: 'bai',
  千: 'qian',
  万: 'wan',
  我: 'wo',
  的: 'de',
  官: 'guan',
  方: 'fang',
  开: 'kai',
  放: 'fang',
  平: 'ping',
  台: 'tai',
  应: 'ying',
  用: 'yong',
  商: 'shang',
  店: 'dian',
  专: 'zhuan',
  业: 'ye',
  企: 'qi',
  个: 'ge',
  人: 'ren',
  户: 'hu',
  快: 'kuai',
  速: 'su',
  智: 'zhi',
  能: 'neng',
  云: 'yun',
  端: 'duan',
  数: 'shu',
  据: 'ju',
  中: 'zhong',
  心: 'xin',
  服: 'fu',
  务: 'wu',
  接: 'jie',
  口: 'kou',
  网: 'wang',
  关: 'guan',
  小: 'xiao',
  程: 'cheng',
  序: 'xu',
  页: 'ye',
  后: 'hou',
  客: 'ke',
  源: 'yuan',
  站: 'zhan',
  目: 'mu',
  录: 'lu',
  审: 'shen',
  核: 'he',
  架: 'jia',
  支: 'zhi',
  付: 'fu',
  微: 'wei',
  信: 'xin',
  宝: 'bao',
  实: 'shi',
  名: 'ming',
  认: 'ren',
  证: 'zheng',
  登: 'deng',
  注: 'zhu',
  册: 'ce',
  短: 'duan',
  邮: 'you',
  箱: 'xiang',
  验: 'yan',
  码: 'ma',
  银: 'yin',
  联: 'lian',
  行: 'hang',
  卡: 'ka',
  订: 'ding',
  单: 'dan',
  会: 'hui',
  员: 'yuan',
  积: 'ji',
  分: 'fen',
  优: 'you',
  惠: 'hui',
  券: 'quan',
  统: 'tong',
  计: 'ji',
  析: 'xi',
  报: 'bao',
  表: 'biao',
  聊: 'liao',
  天: 'tian',
  消: 'xiao',
  息: 'xi',
  通: 'tong',
  知: 'zhi',
  推: 'tui',
  送: 'song',
  上: 'shang',
  传: 'chuan',
  下: 'xia',
  载: 'zai',
  享: 'xiang',
  收: 'shou',
  藏: 'cang',
  评: 'ping',
  论: 'lun',
  点: 'dian',
  赞: 'zan',
  搜: 'sou',
  索: 'suo',
  筛: 'shai',
  选: 'xuan',
  排: 'pai',
  主: 'zhu',
  题: 'ti',
  皮: 'pi',
  肤: 'fu',
  模: 'mo',
  板: 'ban',
  首: 'shou',
  布: 'bu',
  局: 'ju',
  导: 'dao',
  航: 'hang',
  菜: 'cai',
  广: 'guang',
  告: 'gao',
  营: 'ying',
  销: 'xiao',
  插: 'cha',
  件: 'jian',
  工: 'gong',
  具: 'ju',
  助: 'zhu',
  手: 'shou',
  管: 'guan',
  理: 'li',
  安: 'an',
  全: 'quan',
  加: 'jia',
  密: 'mi',
  签: 'qian',
  授: 'shou',
  权: 'quan',
  许: 'xu',
  可: 'ke',
  更: 'geng',
  新: 'xin',
  版: 'ban',
  本: 'ben',
  装: 'zhuang',
  卸: 'xie',
  配: 'pei',
  置: 'zhi',
  设: 'she',
  同: 'tong',
  步: 'bu',
  备: 'bei',
  份: 'fen',
  恢: 'hui',
  复: 'fu',
  入: 'ru',
  出: 'chu',
  打: 'da',
  印: 'yin',
  扫: 'sao',
  描: 'miao',
  地: 'di',
  图: 'tu',
  定: 'ding',
  位: 'wei',
  气: 'qi',
  日: 'ri',
  历: 'li',
  提: 'ti',
  醒: 'xing',
  待: 'dai',
  办: 'ban',
  笔: 'bi',
  记: 'ji',
  文: 'wen',
  档: 'dang',
  视: 'shi',
  频: 'pin',
  音: 'yin',
  直: 'zhi',
  播: 'bo',
  翻: 'fan',
  译: 'yi',
  输: 'shu',
  标: 'biao',
  颜: 'yan',
  色: 'se',
  动: 'dong',
  画: 'hua',
  游: 'you',
  戏: 'xi',
  乐: 'le',
  闻: 'wen',
  社: 'she',
  区: 'qu',
  交: 'jiao',
  钱: 'qian',
  包: 'bao',
  充: 'chong',
  值: 'zhi',
  现: 'xian',
  转: 'zhuan',
  账: 'zhang',
  发: 'fa',
  票: 'piao',
  合: 'he',
  电: 'dian',
  子: 'zi',
  章: 'zhang',
  财: 'cai',
  考: 'kao',
  勤: 'qin',
  批: 'pi',
  流: 'liu',
  公: 'gong',
  招: 'zhao',
  聘: 'pin',
  培: 'pei',
  训: 'xun'
}

const WORD_PINYIN: Record<string, string> = {
  微信支付: 'weixin-zhifu',
  支付宝: 'alipay',
  实名认证: 'shiming-renzheng',
  首页模板: 'shouye-moban'
}

function collapseHyphens(value: string): string {
  return value.replace(/-+/g, '-').replace(/^-|-$/g, '')
}

export function latinSlugFromName(name: string): string {
  let text = String(name || '').normalize('NFKC')
  for (const [word, slug] of Object.entries(WORD_PINYIN)) {
    text = text.replaceAll(word, `-${slug}-`)
  }
  const mapped = [...text]
    .map((ch) => {
      if (PINYIN[ch]) return PINYIN[ch]
      return ch
    })
    .join('')
    .toLowerCase()
  return collapseHyphens(mapped.replace(/[^a-z0-9]+/g, '-')).slice(0, 59)
}

export function isCatalogSlug(value: string): boolean {
  return CATALOG_ID_PATTERN.test(String(value || '').trim())
}

export function isGeneratedFallbackSlug(value: string, kind: CatalogKind): boolean {
  const prefix = FALLBACK_PREFIX[kind]
  return new RegExp(`^${prefix}-[a-z0-9]+$`).test(String(value || '').trim())
}

export function fallbackCatalogSlug(kind: CatalogKind, now = Date.now()): string {
  return `${FALLBACK_PREFIX[kind]}-${now.toString(36)}`
}

export function isStationPackageLocation(value: string): boolean {
  const raw = String(value || '').trim()
  if (!raw) return false
  let path = raw
  if (raw.includes('://')) {
    try {
      path = new URL(raw).pathname
    } catch {
      return false
    }
  }
  return /^\/api\/v1\/public\/source-packages\/[a-f0-9]{64}\.zip$/.test(path)
}

export function isHttpsLocation(value: string): boolean {
  try {
    return new URL(String(value || '').trim()).protocol === 'https:'
  } catch {
    return false
  }
}

export function isTemplateLocation(value: string): boolean {
  const raw = String(value || '').trim()
  if (!raw) return false
  if (raw.toLowerCase().startsWith('https://')) return isHttpsLocation(raw)
  if (raw.includes('..') || raw.startsWith('/')) return false
  return /^[a-zA-Z0-9][a-zA-Z0-9._/-]*$/.test(raw)
}

/**
 * Build a directory id from the display name.
 * Latin/pinyin slugs update as the name changes; Chinese-only names keep a stable fallback.
 */
export function suggestCatalogSlug(
  name: string,
  kind: CatalogKind,
  existingId = '',
  now = Date.now()
): string {
  const latin = latinSlugFromName(name)
  if (isCatalogSlug(latin)) return latin
  if (latin.length >= 2) {
    const padded = latin.padEnd(2, '0')
    return isCatalogSlug(padded) ? padded : fallbackCatalogSlug(kind, now)
  }
  if (isGeneratedFallbackSlug(existingId, kind) || isCatalogSlug(existingId)) {
    return existingId.trim()
  }
  return fallbackCatalogSlug(kind, now)
}
