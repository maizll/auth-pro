import assert from 'node:assert/strict'
import { catalogUploadBlockReason } from './catalog-package-source'

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
