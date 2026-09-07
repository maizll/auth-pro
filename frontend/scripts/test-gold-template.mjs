import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import { createServer } from 'vite'
import vue from '@vitejs/plugin-vue'
import { createSSRApp, h } from 'vue'
import { renderToString } from 'vue/server-renderer'

// Uses existing Vue/Vite only; no browser, network API, or test framework dependency.
const root = fileURLToPath(new URL('../', import.meta.url))
const server = await createServer({
  root,
  configFile: false,
  plugins: [vue()],
  // This isolated SSR test must not launch a background scan of the entire application.
  optimizeDeps: { noDiscovery: true, include: [] },
  logLevel: 'error',
  server: { middlewareMode: true, hmr: false },
  appType: 'custom'
})
const originalWindow = globalThis.window
const render = (component, props = {}, slots = {}) =>
  renderToString(
    createSSRApp({
      render: () => h(component, props, slots)
    })
  )
try {
  const folder = '/src/views/user-panel/login/fintech-gold/'
  const { default: Home } = await server.ssrLoadModule(folder + 'index.vue')
  const { default: Icon } = await server.ssrLoadModule(folder + 'AppIcon.vue')
  const { default: Card } = await server.ssrLoadModule(folder + 'GlassCard.vue')
  const document = JSON.parse(
    await readFile(new URL('./fixtures/fintech-gold.json', import.meta.url), 'utf8')
  )
  const props = {
    document,
    siteName: '模板联调站点',
    siteSubtitle: '现有系统配置',
    loginAction: async () => {}
  }
  const page = await render(Home, props)
  assert.equal([...page.matchAll(/class="[^"]*\bglass-card\b/g)].length, 10)
  assert.match(page, /模板联调站点/)
  assert.match(page, /让每一份软件授权/)
  assert.match(page, /<form[^>]+class="[^"]*query-box/)
  assert.match(page, /<dialog[^>]+aria-labelledby="login-title"/)
  assert.ok([...page.matchAll(/<svg\b/g)].length > 25, 'Icons must render offline, including SSR')
  assert.match(page, /--gold:#f0b90b/)

  const labeled = await render(Icon, { name: 'search', label: '搜索' })
  assert.match(labeled, /aria-label="搜索"/)
  assert.doesNotMatch(labeled, /aria-hidden="true"/)
  assert.equal(
    await render(Icon, { name: 'ri:rocket-2-line' }),
    await render(Icon, { name: 'rocket' })
  )
  const fallback = await render(Icon, { name: 'shield-check' })
  for (const name of ['unknown', '__proto__', 'constructor', 'toString']) {
    assert.equal(await render(Icon, { name }), fallback)
  }
  const unsafe = await render(Home, {
    ...props,
    document: {
      ...document,
      theme: { primaryColor: 'url(https://example.invalid/unsafe)' },
      hero: { title: '<script>alert(1)</script>' }
    }
  })
  assert.doesNotMatch(unsafe, /<script>|example\.invalid/)
  assert.match(unsafe, /&lt;script&gt;/)
  assert.match(unsafe, /--gold:#f0b90b/)

  let handlers
  const CapturedCard = {
    ...Card,
    setup(props, context) {
      handlers = Card.setup(props, context)
      return handlers
    }
  }
  const media = { matches: true }
  globalThis.window = { matchMedia: () => media }
  await render(CapturedCard, { interactive: true })
  const values = new Map()
  const event = {
    pointerType: 'mouse',
    clientX: 60,
    clientY: 70,
    currentTarget: {
      getBoundingClientRect: () => ({ left: 10, top: 20, width: 200, height: 100 }),
      style: {
        setProperty: (key, value) => values.set(key, value),
        removeProperty: (key) => values.delete(key)
      }
    }
  }
  handlers.moveGlow(event)
  assert.deepEqual(Object.fromEntries(values), { '--glow-x': '25%', '--glow-y': '50%' })
  handlers.resetGlow(event)
  handlers.moveGlow({ ...event, pointerType: 'touch' })
  assert.equal(values.size, 0)
  media.matches = false
  handlers.moveGlow(event)
  assert.equal(values.size, 0, 'Reduced motion must disable pointer effects')
  const staticCard = await render(CapturedCard, { as: 'form', interactive: false })
  assert.match(staticCard, /^<form/)
  assert.doesNotMatch(staticCard, /glass-card--interactive/)

  const source = await readFile(
    new URL('../src/views/user-panel/login/fintech-gold/index.vue', import.meta.url),
    'utf8'
  )
  assert.match(source, /<style scoped>/)
  assert.doesNotMatch(
    source,
    /^\s*(?::root|body|html)\s*\{/m,
    'Template CSS must not reset the host globally'
  )
  assert.match(source, /props\.loginAction/)
  assert.doesNotMatch(
    source,
    /fetchActiveTemplate|fetchSystemConfig|localStorage\.setItem/,
    'The host retains API/login ownership'
  )
  console.log(
    'PASS: gold layout, real template fixture, offline icons, safe theme, shared login boundary, scoped CSS and accessible motion'
  )
} finally {
  if (originalWindow === undefined) delete globalThis.window
  else globalThis.window = originalWindow
  await server.close()
}
