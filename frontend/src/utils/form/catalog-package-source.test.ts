import assert from 'node:assert/strict'
import { catalogUploadBlockReason, developerCatalogBlockReason } from './catalog-package-source'

const github =
  'https://github.com/maizll/authproPlus-source/releases/download/alipay-f2f-1.0.0/alipay-f2f-1.0.0.zip'

assert.equal(
  catalogUploadBlockReason({
    appId: 1,
    source: 'public',
    push: false,
    hasFile: false,
    location: github,
    priceYuan: '0'
  }),
  ''
)
assert.equal(
  catalogUploadBlockReason({
    appId: 1,
    source: 'public',
    push: false,
    hasFile: false,
    location: '',
    priceYuan: '0'
  }),
  '请填写 https 公开地址'
)
assert.equal(
  catalogUploadBlockReason({
    appId: 1,
    source: 'public',
    push: false,
    hasFile: false,
    location: 'http://github.com/maizll/authproPlus-source/a.zip',
    priceYuan: '0'
  }),
  '外部地址须以 https:// 开头'
)
assert.equal(
  catalogUploadBlockReason({
    appId: 1,
    source: 'public',
    push: false,
    hasFile: false,
    location: github,
    priceYuan: '19.9'
  }),
  '公开地址只能用于免费条目。收费请改用私有 GitHub 仓库，或上传压缩包'
)
assert.equal(
  catalogUploadBlockReason({
    appId: 1,
    source: 'github',
    push: false,
    hasFile: false,
    location: github,
    priceYuan: '19.9'
  }),
  ''
)
assert.equal(
  catalogUploadBlockReason({
    appId: 1,
    source: 'github',
    push: false,
    hasFile: false,
    location: github,
    priceYuan: '0'
  }),
  '私有 GitHub 仓库用于收费条目，请填写大于 0 的售价'
)
assert.equal(
  catalogUploadBlockReason({
    appId: 1,
    source: 'upload',
    push: false,
    hasFile: true,
    location: '',
    priceYuan: '10'
  }),
  ''
)
assert.equal(
  catalogUploadBlockReason({
    appId: 1,
    source: 'upload',
    push: true,
    hasFile: false,
    location: '',
    priceYuan: '0'
  }),
  '请选择压缩包'
)
assert.equal(
  catalogUploadBlockReason({
    appId: 0,
    source: 'public',
    push: false,
    hasFile: false,
    location: github,
    priceYuan: '0'
  }),
  '请选择应用'
)

assert.equal(
  developerCatalogBlockReason({
    kind: 'plugin',
    source: 'public',
    location: '',
    sha256: '',
    priceYuan: '0',
    requirePackage: false
  }),
  ''
)
assert.equal(
  developerCatalogBlockReason({
    kind: 'plugin',
    source: 'public',
    location: github,
    sha256: '',
    priceYuan: '19.9',
    requirePackage: false
  }),
  '公开地址只能用于免费条目。收费请改用私有 GitHub 仓库，或上传压缩包'
)
assert.equal(
  developerCatalogBlockReason({
    kind: 'plugin',
    source: 'github',
    location: github,
    sha256: '',
    priceYuan: '19.9',
    requirePackage: true
  }),
  ''
)
assert.equal(
  developerCatalogBlockReason({
    kind: 'template',
    source: 'github',
    location: '',
    sha256: '',
    priceYuan: '0',
    requirePackage: false
  }),
  '私有 GitHub 仓库用于收费条目，请填写大于 0 的售价'
)
assert.equal(
  developerCatalogBlockReason({
    kind: 'plugin',
    source: 'upload',
    location: '',
    sha256: '',
    priceYuan: '0',
    requirePackage: false
  }),
  '免费条目请改用公开地址。上传压缩包用于本站托管的收费条目'
)
assert.equal(
  developerCatalogBlockReason({
    kind: 'template',
    source: 'public',
    location: 'templates/demo-home.json',
    sha256: 'ab'.repeat(32),
    priceYuan: '0',
    requirePackage: true
  }),
  ''
)
assert.equal(
  developerCatalogBlockReason({
    kind: 'plugin',
    source: 'public',
    location: github,
    sha256: '',
    priceYuan: '0',
    requirePackage: true
  }),
  '提交审核前请填写校验码'
)
