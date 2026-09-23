import assert from 'node:assert/strict'
import {
  isCatalogSlug,
  isHttpsLocation,
  isStationPackageLocation,
  isTemplateLocation,
  latinSlugFromName,
  suggestCatalogSlug,
  fallbackCatalogSlug,
  SHA256_HEX_PATTERN
} from './catalog-slug'

assert.equal(latinSlugFromName('My Plugin'), 'my-plugin')
assert.equal(latinSlugFromName('Demo_SDK v2'), 'demo-sdk-v2')
assert.equal(latinSlugFromName('微信支付'), 'weixin-zhifu')
assert.equal(latinSlugFromName('支付宝'), 'alipay')
assert.equal(latinSlugFromName('支付插件'), 'zhifuchajian')
assert.equal(latinSlugFromName('首页模板'), 'shouye-moban')
assert.equal(latinSlugFromName('ＡＢＣ插件'), 'abcchajian')
assert.equal(latinSlugFromName('你好世界'), '')
assert.equal(suggestCatalogSlug('Demo SDK', 'plugin'), 'demo-sdk')
assert.equal(
  suggestCatalogSlug('你好世界', 'plugin', '', 1_234_567_890),
  fallbackCatalogSlug('plugin', 1_234_567_890)
)
assert.equal(suggestCatalogSlug('你好世界', 'template', '', 99), 'template-2r')
assert.ok(isCatalogSlug('plugin-abc'))
assert.equal(isCatalogSlug('P'), false)
assert.ok(SHA256_HEX_PATTERN.test('a'.repeat(64)))
assert.equal(SHA256_HEX_PATTERN.test('zz'), false)
assert.equal(
  isStationPackageLocation('/api/v1/public/source-packages/' + 'ab'.repeat(32) + '.zip'),
  true
)
assert.equal(isStationPackageLocation('https://cdn.example.com/a.zip'), false)
assert.equal(isHttpsLocation('https://cdn.example.com/a.zip'), true)
assert.equal(isHttpsLocation('http://cdn.example.com/a.zip'), false)
assert.equal(isTemplateLocation('templates/demo-home.json'), true)
assert.equal(isTemplateLocation('../secret.json'), false)

console.log('catalog-slug ok')
