import { execFileSync } from 'node:child_process'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

// 两种用法：
//   --release-info <包内 release.json> <official|client> <latest.json> <releases.json> <版本> <发布时间>
//     打包前调用，只算更新说明，写进包内 backend/release.json，随包一起签名（上传更新包时据此展示版本、说明和适用端）。
//   <latest.json> <releases.json> <版本> <发布时间> <文件名> <下载地址> <SHA256> <大小> <releases 地址>
//     打包后调用，写在线更新用的 latest.json 和 releases.json。
const infoMode = process.argv[2] === '--release-info'
const [infoPath, edition] = infoMode ? process.argv.slice(3, 5) : []
const [
  latestPath,
  releasesPath,
  version,
  releasedAt,
  packageFileName,
  packageUrl,
  packageSha256,
  packageSizeText,
  releasesUrl
] = process.argv.slice(infoMode ? 5 : 2)

if (infoMode) {
  if (!infoPath || !latestPath || !releasesPath || !version || !releasedAt) {
    throw new Error('Missing release info arguments')
  }
  if (edition !== 'official' && edition !== 'client') {
    throw new Error(`Package edition must be official or client: ${edition}`)
  }
} else if (!latestPath || !releasesPath || !version || !releasedAt || !packageFileName || !packageUrl || !packageSha256 || !packageSizeText || !releasesUrl) {
  throw new Error('Missing release manifest arguments')
}

if (!/^\d+\.\d+\.\d+$/.test(version)) {
  throw new Error(`Version must match X.Y.Z: ${version}`)
}
if (Number.isNaN(Date.parse(releasedAt))) {
  throw new Error(`Invalid release time: ${releasedAt}`)
}
if (!infoMode) {
  if (packageFileName !== `auth_pro-full-v${version}.tar.gz`) {
    throw new Error(`Unexpected package file name: ${packageFileName}`)
  }
  if (!/^[a-f0-9]{64}$/i.test(packageSha256)) {
    throw new Error('Package SHA256 must contain 64 hexadecimal characters')
  }
  for (const [label, value] of [['package', packageUrl], ['releases', releasesUrl]]) {
    const parsed = new URL(value)
    if (parsed.protocol !== 'https:') {
      throw new Error(`${label} URL must use HTTPS`)
    }
  }
}

const packageSize = infoMode ? 0 : Number(packageSizeText)
if (!infoMode && (!Number.isSafeInteger(packageSize) || packageSize <= 0)) {
  throw new Error(`Invalid package size: ${packageSizeText}`)
}

const readJson = (filePath) => {
  if (!fs.existsSync(filePath)) return null
  return JSON.parse(fs.readFileSync(filePath, 'utf8').replace(/^\uFEFF/, ''))
}

const normalizeNotes = (value) => {
  if (!Array.isArray(value)) return []
  return value.map((item) => String(item).trim()).filter(Boolean)
}

const parseConfiguredNotes = () => {
  const value = (process.env.AUTO_PRO_RELEASE_NOTES || '').trim()
  if (!value) return []
  if (value.startsWith('[')) return normalizeNotes(JSON.parse(value))
  return normalizeNotes(value.split(/\r?\n/))
}

const repositoryRoot = process.env.AUTO_PRO_RELEASE_REPOSITORY_DIR
  ? path.resolve(process.env.AUTO_PRO_RELEASE_REPOSITORY_DIR)
  : path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')

const runGit = (args) => {
  try {
    return execFileSync('git', args, {
      cwd: repositoryRoot,
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'ignore']
    }).trim()
  } catch {
    return ''
  }
}

const semanticVersionParts = (value) => {
  const match = String(value).trim().match(/^v?(\d+)\.(\d+)\.(\d+)$/)
  return match ? match.slice(1).map(Number) : null
}

const compareSemanticVersions = (left, right) => {
  const leftParts = semanticVersionParts(left)
  const rightParts = semanticVersionParts(right)
  if (!leftParts || !rightParts) return 0
  for (let index = 0; index < leftParts.length; index += 1) {
    if (leftParts[index] !== rightParts[index]) return leftParts[index] - rightParts[index]
  }
  return 0
}

const gitRefExists = (ref) => Boolean(ref && !ref.startsWith('-') && runGit(['rev-parse', '--verify', '--quiet', `${ref}^{commit}`]))

const collectGitNotes = (previousReleases) => {
  if (!gitRefExists('HEAD')) return []

  const configuredCurrentRef = (process.env.AUTO_PRO_RELEASE_CURRENT_REF || '').trim()
  const currentRef = gitRefExists(configuredCurrentRef) ? configuredCurrentRef : 'HEAD'
  const configuredBaseRef = (process.env.AUTO_PRO_RELEASE_BASE_REF || '').trim()
  let baseRef = gitRefExists(configuredBaseRef) ? configuredBaseRef : ''

  if (!baseRef) {
    const releaseRefs = previousReleases
      .map((item) => String(item?.version || '').trim())
      .filter((candidate) => semanticVersionParts(candidate) && compareSemanticVersions(candidate, version) < 0)
      .map((candidate) => `v${candidate.replace(/^v/, '')}`)
    const repositoryTags = runGit(['tag', '--merged', currentRef, '--list', 'v*'])
      .split(/\r?\n/)
      .map((tag) => tag.trim())
      .filter((tag) => semanticVersionParts(tag) && compareSemanticVersions(tag, version) < 0)
    baseRef = [...new Set([...releaseRefs, ...repositoryTags])]
      .filter(gitRefExists)
      .sort((left, right) => compareSemanticVersions(right, left))[0] || ''
  }

  const revision = baseRef ? `${baseRef}..${currentRef}` : currentRef
  return normalizeNotes(runGit(['log', '--no-merges', '--reverse', '--format=%s', revision]).split(/\r?\n/))
}

const previousLatest = readJson(latestPath)
const previousHistory = readJson(releasesPath)
const previousReleases = Array.isArray(previousHistory)
  ? previousHistory
  : Array.isArray(previousHistory?.releases)
    ? previousHistory.releases
    : []

const previousVersion = previousReleases.find((item) => String(item?.version || '').trim() === version)
const configuredNotes = parseConfiguredNotes()
const gitNotes = collectGitNotes(previousReleases)
if (process.env.AUTO_PRO_REQUIRE_GIT_RELEASE_NOTES === '1' && !configuredNotes.length && !gitNotes.length) {
  throw new Error('No Git release notes found for this version')
}
const notes = configuredNotes.length
  ? configuredNotes
  : gitNotes.length
    ? gitNotes
    : normalizeNotes(previousVersion?.notes).length
      ? normalizeNotes(previousVersion.notes)
      : String(previousLatest?.version || '').trim() === version && normalizeNotes(previousLatest?.notes).length
        ? normalizeNotes(previousLatest.notes)
        : [`版本 ${version} 更新`]

const channel = (process.env.AUTO_PRO_RELEASE_CHANNEL || 'stable').trim() || 'stable'
const minVersion = (process.env.AUTO_PRO_MIN_VERSION || '0.0.0').trim() || '0.0.0'
const release = { version, channel, releasedAt, notes }

const writeJsonAtomic = (filePath, value) => {
  const temporaryPath = `${filePath}.tmp`
  fs.mkdirSync(path.dirname(filePath), { recursive: true })
  fs.writeFileSync(temporaryPath, `${JSON.stringify(value, null, 2)}\n`, 'utf8')
  fs.renameSync(temporaryPath, filePath)
}

if (infoMode) {
  writeJsonAtomic(infoPath, { version, edition, channel, minVersion, releasedAt, notes })
  process.exit(0)
}

const versionParts = (value) => {
  const matches = String(value).replace(/^v/, '').match(/\d+/g)
  return matches ? matches.map(Number) : []
}

const compareVersionsDesc = (left, right) => {
  const leftParts = versionParts(left.version)
  const rightParts = versionParts(right.version)
  const length = Math.max(leftParts.length, rightParts.length)
  for (let index = 0; index < length; index += 1) {
    const difference = (rightParts[index] || 0) - (leftParts[index] || 0)
    if (difference) return difference
  }
  return String(right.releasedAt || '').localeCompare(String(left.releasedAt || ''))
}

const releases = [
  release,
  ...previousReleases
    .filter((item) => String(item?.version || '').trim() !== version)
    .map((item) => ({
      version: String(item?.version || '').trim(),
      channel: String(item?.channel || 'stable').trim() || 'stable',
      releasedAt: String(item?.releasedAt || '').trim(),
      notes: normalizeNotes(item?.notes)
    }))
    .filter((item) => item.version)
].sort(compareVersionsDesc)

const latest = {
  version,
  channel,
  minVersion,
  force: false,
  releasedAt,
  releasesUrl,
  package: {
    os: 'linux',
    arch: 'amd64',
    fileName: packageFileName,
    url: packageUrl,
    sha256: packageSha256,
    size: packageSize,
    signature: `sha256:${packageSha256.toLowerCase()}`
  },
  actions: {
    updateFrontend: true,
    updateBackend: true,
    restartBackend: true,
    backupDatabase: true
  },
  notes
}

writeJsonAtomic(latestPath, latest)
writeJsonAtomic(releasesPath, { releases })