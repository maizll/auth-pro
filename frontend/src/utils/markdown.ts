// 官网文档的 Markdown 渲染。先转义 HTML，再交给已有的 marked，避免正文插入脚本。
import { marked } from 'marked'

export interface SiteTocItem {
  id: string
  text: string
  level: number
}

function stripMatchingTitle(source: string, pageTitle: string) {
  const title = pageTitle.trim()
  if (!title) return source
  const match = /^\s*#\s+(.+?)\s*(?:\r?\n|$)/.exec(source)
  if (!match) return source
  const heading = match[1].replace(/[`*_]/g, '').trim()
  if (heading !== title) return source
  return source.slice(match[0].length).replace(/^\s+/, '')
}

function siteMarkdownToc(source: string): SiteTocItem[] {
  const items: SiteTocItem[] = []
  const seen = new Map<string, number>()
  for (const raw of source.split('\n')) {
    const match = /^(#{1,4})\s+(.+)$/.exec(raw.trim())
    if (!match) continue
    const text = match[2].replace(/[`*]/g, '').trim()
    let id = text
      .toLowerCase()
      .replace(/\s+/g, '-')
      .replace(/[^\w\u4e00-\u9fff-]/g, '')
    if (!id) id = 'section'
    const count = seen.get(id) || 0
    seen.set(id, count + 1)
    if (count > 0) id = `${id}-${count + 1}`
    items.push({ id, text, level: match[1].length })
  }
  return items
}

/** 输入 Markdown 原文，输出可插入页面的 HTML 和目录。原文里的标签会被转义。页面已有同名大标题时去掉正文第一行的一级标题。 */
export function renderSiteMarkdown(
  source: string,
  pageTitle = ''
): { html: string; toc: SiteTocItem[] } {
  const body = stripMatchingTitle(source, pageTitle)
  const toc = siteMarkdownToc(body)
  const parsed = marked.parse(body.replace(/</g, '&lt;'), { gfm: true, breaks: false })
  let index = 0
  const html = String(parsed).replace(/<h([1-4])>/g, (_match, level: string) => {
    index += 1
    const id = toc[index - 1]?.id || `section-${index}`
    return `<h${level} id="${id}">`
  })
  return { html, toc }
}
