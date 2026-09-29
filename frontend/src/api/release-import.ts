// 应用发布、插件发布和模板发布共用的仓库导入接口。
import request from '@/utils/http'

export interface ReleaseImportItem {
  tag: string
  title: string
  changelog: string
  assetName: string
  publishedAt: string
}

export interface ReleaseImportResult {
  stagingId: string
  version: string
  title: string
  changelog: string
  fileName: string
  fileSizeBytes: number
  fileMd5: string
  fileSha256: string
  assetName: string
}

export function fetchReleaseImports(
  apiBase: string,
  purpose: string,
  appId: number,
  priceCents = 0
) {
  return request.post<{ repo: string; releases: ReleaseImportItem[]; empty?: string }>({
    url: `${apiBase}/releases`,
    data: { purpose, appId, priceCents },
    showSuccessMessage: false
  })
}

export function fetchReleaseAsset(
  apiBase: string,
  data: {
    purpose: string
    appId: number
    priceCents?: number
    appKey?: string
    tag: string
    assetName: string
  }
) {
  return request.post<ReleaseImportResult>({
    url: `${apiBase}/fetch`,
    data,
    showSuccessMessage: false
  })
}

export function probeReleaseUrl(apiBase: string, url: string) {
  return request.post<ReleaseImportResult>({
    url: `${apiBase}/probe-url`,
    data: { url },
    showSuccessMessage: false
  })
}

export function materializeRelease(
  apiBase: string,
  data: { stagingId: string; kind: string; itemId: string; version: string; priceCents: number }
) {
  return request.post<{ location: string; sha256: string; fileMd5: string; fileSizeBytes: number }>(
    {
      url: `${apiBase}/materialize`,
      data,
      showSuccessMessage: false
    }
  )
}
