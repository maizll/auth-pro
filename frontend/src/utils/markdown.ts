// 官网文档的 Markdown 渲染。先保住代码块里的原文，再转义正文里的标签，避免插入脚本。
// 代码高亮只用 highlight.js 的核心和下面几种语言，不打包整库。
import hljs from 'highlight.js/lib/core'
import bash from 'highlight.js/lib/languages/bash'
import go from 'highlight.js/lib/languages/go'
import javascript from 'highlight.js/lib/languages/javascript'
import json from 'highlight.js/lib/languages/json'
import nginx from 'highlight.js/lib/languages/nginx'
import php from 'highlight.js/lib/languages/php'
import xml from 'highlight.js/lib/languages/xml'
import yaml from 'highlight.js/lib/languages/yaml'
import { Marked, type Tokens } from 'marked'

export interface SiteTocItem {
  id: string
  text: string
  level: number
}

hljs.registerLanguage('bash', bash)
hljs.registerLanguage('go', go)
hljs.registerLanguage('javascript', javascript)
hljs.registerLanguage('json', json)
hljs.registerLanguage('nginx', nginx)
hljs.registerLanguage('php', php)
hljs.registerLanguage('xml', xml)
hljs.registerLanguage('yaml', yaml)
hljs.registerAliases(['sh', 'shell', 'zsh'], { languageName: 'bash' })
hljs.registerAliases(['js'], { languageName: 'javascript' })
hljs.registerAliases(['yml'], { languageName: 'yaml' })
hljs.registerAliases(['html', 'htm'], { languageName: 'xml' })

const siteMarked = new Marked({ gfm: true, breaks: false })
siteMarked.use({
  renderer: {
    code({ text, lang }: Tokens.Code) {
      return renderSiteCodeBlock(text, lang)
    },
    html({ text }: Tokens.HTML | Tokens.Tag) {
      return escapeHtml(text)
    }
  }
})

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

function escapeHtml(value: string) {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

// 围栏代码保持原样，其余正文里的 < 先转义，避免把 <端口> 这类字样吃成标签。
function escapeProseAngles(source: string) {
  const blocks: string[] = []
  const parked = source.replace(/```[\s\S]*?```/g, (block) => {
    const token = `@@CODE${blocks.length}@@`
    blocks.push(block)
    return token
  })
  const escaped = parked.replace(/</g, '&lt;')
  return escaped.replace(/@@CODE(\d+)@@/g, (_match, index: string) => blocks[Number(index)] || '')
}

function highlightSource(text: string, language: string) {
  if (language && hljs.getLanguage(language)) {
    return hljs.highlight(text, { language, ignoreIllegals: true }).value
  }
  return escapeHtml(text)
}

function renderSiteCodeBlock(text: string, lang?: string) {
  const language = (lang || '').trim().split(/\s+/)[0]?.toLowerCase() || ''
  const highlighted = highlightSource(text.replace(/\n$/, ''), language)
  const className = language ? `hljs language-${escapeHtml(language)}` : 'hljs'
  return `<div class="site-code"><button type="button" class="site-code__copy">复制</button><pre><code class="${className}">${highlighted}</code></pre></div>`
}

/** 输入 Markdown 原文，输出可插入页面的 HTML 和目录。页面已有同名大标题时去掉正文第一行的一级标题。 */
export function renderSiteMarkdown(
  source: string,
  pageTitle = ''
): { html: string; toc: SiteTocItem[] } {
  const body = stripMatchingTitle(source, pageTitle)
  const toc = siteMarkdownToc(body)
  const parsed = siteMarked.parse(escapeProseAngles(body))
  let index = 0
  const html = String(parsed).replace(/<h([1-4])>/g, (_match, level: string) => {
    index += 1
    const id = toc[index - 1]?.id || `section-${index}`
    return `<h${level} id="${id}">`
  })
  return { html, toc }
}
