import assert from 'node:assert/strict'
import { mailPreviewDocument } from './mail-preview'

// 纯文本邮件里的尖括号必须转义，HTML 邮件放进带 CSP 的文档（沙箱 iframe 里另外禁止脚本）。
const text = mailPreviewDocument('你好 <img src=x onerror=alert(1)>\n第二行', 'text')
assert.ok(text.includes('你好 &lt;img src=x onerror=alert(1)&gt;<br />第二行'))
assert.ok(!text.includes('<img'))

const html = mailPreviewDocument('<p>开通成功</p>', 'html')
assert.ok(html.includes("default-src 'none'"))
assert.ok(html.includes('<body><p>开通成功</p></body>'))

console.log('mail-preview ok')
