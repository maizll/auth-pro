// 生成邮件预览 iframe 的 srcdoc。沙箱本身已禁止脚本；这里再加一层内容安全策略，
// 不让预览去加载外部图片和样式（避免打开预览就向第三方报到）。纯文本邮件先转义再换行。
const PREVIEW_CSP =
  "default-src 'none'; img-src data:; style-src 'unsafe-inline'; font-src data:; form-action 'none'"

function escapeMailText(text: string): string {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

export function mailPreviewDocument(content: string, contentType: 'text' | 'html'): string {
  const body = contentType === 'html' ? content : escapeMailText(content).replace(/\n/g, '<br />')
  return (
    '<!doctype html><html><head><meta charset="utf-8">' +
    `<meta http-equiv="Content-Security-Policy" content="${PREVIEW_CSP}">` +
    '<style>body{margin:0;padding:16px;font:14px/1.7 -apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;color:#1f2329;word-break:break-word}</style>' +
    `</head><body>${body}</body></html>`
  )
}
