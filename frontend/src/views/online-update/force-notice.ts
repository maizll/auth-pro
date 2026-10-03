// 强制更新提醒的共享状态：后台横幅、登录后弹框和在线更新页都读这一份，检查更新或改开关后刷新。
import { reactive } from 'vue'
import { fetchOnlineUpdateNotice, type OnlineUpdateNotice } from '@/api/update'

export const forceNotice = reactive<{ data: OnlineUpdateNotice | null }>({ data: null })

export async function refreshForceNotice() {
  try {
    forceNotice.data = await fetchOnlineUpdateNotice()
  } catch {
    // 读不到时不提示，也不打扰正常使用
  }
}

export function setForceNotice(data: OnlineUpdateNotice | null | undefined) {
  if (data) forceNotice.data = data
}
