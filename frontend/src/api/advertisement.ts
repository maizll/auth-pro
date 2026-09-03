import request from '@/utils/http'

/** 广告位标识，与投放方约定的三处位置一一对应 */
export type AdPosition = 'home-banner' | 'sidebar' | 'popup'

/** 一条投放内容；imageUrl 是带签名的临时地址，随时可能过期，渲染层需要兜住加载失败 */
export interface AdvertisementItem {
  id: string
  title: string
  imageUrl: string
  destinationUrl: string
  position: string
  weight: number
  startAt: string
  endAt: string
  description: string
  /** 无投放数据时由前端合成的占位项，不来自接口 */
  isPlaceholder?: boolean
}

/**
 * 投放内容走后端代理：上游未开放 CORS，且共用 axios 实例会附带后台 JWT，
 * 不能把它指向第三方域名。失败不弹全局提示——广告位会自行退回占位。
 */
export function fetchAdvertisements(position: AdPosition) {
  return request.get<{ records: AdvertisementItem[] }>({
    url: '/api/advertisements',
    params: { position },
    showErrorMessage: false
  })
}
