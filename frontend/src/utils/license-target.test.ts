import assert from 'node:assert/strict'
import { licenseTargetError } from './license-target'

assert.equal(licenseTargetError('domain', ''), '请填写授权目标')
assert.equal(licenseTargetError('domain', 'not a domain'), '单域名格式不正确')
assert.equal(licenseTargetError('domain', 'shop.example.com'), '')
assert.equal(licenseTargetError('wildcard', 'example.com'), '泛域名格式不正确')
assert.equal(licenseTargetError('wildcard', '*.example.com'), '')
assert.equal(licenseTargetError('ip', '10.1.1'), 'IP 格式不正确')
assert.equal(licenseTargetError('ip', '192.168.1.1'), '')
