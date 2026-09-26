import assert from 'node:assert/strict'
import { catalogUploadBlockReason } from './catalog-package-source'

const github =
  'https://github.com/maizll/authproPlus-source/releases/download/alipay-f2f-1.0.0/alipay-f2f-1.0.0.zip'

assert.equal(
  catalogUploadBlockReason({ appId: 1, push: false, hasFile: false, location: github }),
  ''
)
assert.equal(
  catalogUploadBlockReason({ appId: 1, push: false, hasFile: false, location: '' }),
  '请上传压缩包，或填写 https 外部地址（二选一）'
)
assert.equal(
  catalogUploadBlockReason({
    appId: 1,
    push: false,
    hasFile: false,
    location: 'http://github.com/maizll/authproPlus-source/a.zip'
  }),
  '外部地址须以 https:// 开头'
)
assert.equal(
  catalogUploadBlockReason({ appId: 1, push: true, hasFile: false, location: github }),
  '推送 Release 时请先选择压缩包'
)
assert.equal(catalogUploadBlockReason({ appId: 1, push: true, hasFile: true, location: '' }), '')
assert.equal(
  catalogUploadBlockReason({ appId: 1, push: false, hasFile: true, location: github }),
  ''
)
assert.equal(
  catalogUploadBlockReason({ appId: 1, push: false, hasFile: true, location: '' }),
  '未推送 Release 时请填写 https 外部地址'
)
assert.equal(
  catalogUploadBlockReason({ appId: 0, push: false, hasFile: false, location: github }),
  '请选择应用'
)
