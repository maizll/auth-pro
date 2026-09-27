#!/usr/bin/env node
// 对照 frontend/knip-unused-baseline.json。出现基线里没有的未使用导出就失败。
// 比基线更少可以通过；删掉导出后应更新基线，避免旧名字以后又能加回来。
import { execFileSync } from 'node:child_process'
import { readFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const frontend = path.join(root, 'frontend')
const baselinePath = path.join(frontend, 'knip-unused-baseline.json')
const knipBin = path.join(frontend, 'node_modules', 'knip', 'bin', 'knip.js')

function collect(issues) {
  const names = []
  for (const issue of issues || []) {
    const file = String(issue.file || '').replaceAll('\\', '/')
    for (const item of issue.exports || []) {
      if (item?.name) names.push(`${file}:${item.name}`)
    }
    for (const item of issue.types || []) {
      if (item?.name) names.push(`${file}:type:${item.name}`)
    }
    const members = issue.enumMembers
    if (Array.isArray(members)) {
      for (const item of members) {
        if (item?.name) names.push(`${file}:${item.name}`)
      }
    } else if (members && typeof members === 'object') {
      for (const [enumName, list] of Object.entries(members)) {
        for (const item of list || []) {
          if (item?.name) names.push(`${file}:${enumName}.${item.name}`)
        }
      }
    }
  }
  return [...new Set(names)].sort()
}

function parseKnipStdout(text) {
  const start = text.indexOf('{')
  if (start < 0) throw new Error('knip 没有输出 JSON')
  return JSON.parse(text.slice(start))
}

let raw = ''
try {
  raw = execFileSync(process.execPath, [knipBin, '--reporter', 'json', '--no-progress'], {
    cwd: frontend,
    encoding: 'utf8'
  })
} catch (error) {
  raw = error.stdout?.toString() || ''
  if (!raw.trim()) {
    process.stderr.write(error.stderr?.toString() || error.message)
    process.exit(1)
  }
}

const report = parseKnipStdout(raw)
const current = collect(report.issues)
const baseline = JSON.parse(readFileSync(baselinePath, 'utf8'))
const allowed = new Set(baseline.unusedExports || [])
const added = current.filter((name) => !allowed.has(name))
const removed = [...allowed].filter((name) => !current.includes(name)).sort()

if (removed.length) {
  console.log(`基线里有 ${removed.length} 个导出已经不再未使用，请从 knip-unused-baseline.json 删掉：`)
  for (const name of removed) console.log(`  - ${name}`)
}
if (added.length) {
  console.error(`新增 ${added.length} 个未使用导出：`)
  for (const name of added) console.error(`  + ${name}`)
  process.exit(1)
}
console.log(`未使用导出 ${current.length} 个，均在基线内`)
