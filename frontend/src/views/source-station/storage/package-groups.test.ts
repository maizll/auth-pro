import assert from 'node:assert/strict'
import {
  classifyTag,
  compareVersionDesc,
  filterPackageRows,
  groupPackageFiles,
  type PackageFile
} from './package-groups'

// 与线上收费仓库一致的样子：客户端每版一个安装包加 latest.json，同一插件新旧两种标签各一份。
const file = (tag: string, name: string, extra: Partial<PackageFile> = {}): PackageFile => ({
  key: `acme/paid/${tag}/${name}`,
  name,
  size: name.endsWith('.json') ? 300 : 9000,
  tag,
  ...extra
})
const files = [
  file('client/v1.8.7', 'auth_pro-full-v1.8.7.tar.gz', { item: 'auth-pro 1.8.7' }),
  file('client/v1.8.7', 'latest.json'),
  file('client/v1.7.10', 'auth_pro-full-v1.7.10.tar.gz'),
  file('client/v1.8.10', 'latest.json'),
  file('client/v1.8.10', 'auth_pro-full-v1.8.10.tar.gz'),
  file('plugins/paid/alipay-f2f-1.0.0', 'alipay-f2f-1.0.0.zip', { item: 'alipay-f2f 1.0.0' }),
  file('paid-plugin-alipay-f2f-1.0.0', 'alipay-f2f-1.0.0.zip', { orphan: true }),
  file('templates/paid/gold-shop-2.1.0', 'gold-shop-2.1.0.zip'),
  file('free-plugin-hello-1.0.0', 'hello-1.0.0.zip')
]

const groups = groupPackageFiles(files)
assert.deepEqual(
  groups.client.map((row) => row.version),
  ['v1.8.10', 'v1.8.7', 'v1.7.10'],
  '客户端版本按版本号倒序，不按字符串'
)
const v187 = groups.client[1]
assert.equal(v187.files.length, 2, '安装包和 latest.json 合并成一行')
assert.equal(v187.main.name, 'auth_pro-full-v1.8.7.tar.gz', '主文件是安装包')
assert.ok(v187.hasManifest)
assert.equal(v187.item, 'auth-pro 1.8.7')

assert.equal(groups.plugin.length, 2, '新旧两种标签各一行')
const legacy = groups.plugin.find((row) => row.legacy)
assert.ok(legacy && legacy.orphan && legacy.name === 'alipay-f2f' && legacy.version === '1.0.0')
assert.equal(groups.template[0].name, 'gold-shop')
assert.equal(groups.other[0].tag, 'free-plugin-hello-1.0.0')

assert.deepEqual(classifyTag('paid-template-shop-1.2.3-beta.1'), {
  group: 'template',
  name: 'shop',
  version: '1.2.3-beta.1',
  legacy: true
})
assert.ok(compareVersionDesc('1.10.0', '1.9.9') < 0)

assert.equal(filterPackageRows(groups.client, '1.8.7').length, 1)
assert.equal(filterPackageRows(groups.plugin, 'ALIPAY').length, 2, '搜索不分大小写')
assert.equal(filterPackageRows(groups.client, 'latest').length, 2, '文件名也能搜')

// 没有标签的存储（对象存储、WebDAV）按目录归组。
const flat = groupPackageFiles([{ key: 'packages/demo/a.zip', name: 'a.zip', size: 1 }])
assert.equal(flat.other[0].tag, 'packages/demo')

console.log('package-groups ok')
