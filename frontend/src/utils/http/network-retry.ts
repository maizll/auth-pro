/**
 * 连不上服务器时的提示：大白话 + 「重试」按钮。
 * 幂等的 GET 请求先自动重试一次，还不行才弹这个提示；点「重试」就再发一次，原来的调用方直接拿到结果。
 */
import { h } from 'vue'

/** 提示停留时间。到时间没点「重试」就按失败返回。 */
const NETWORK_RETRY_PROMPT_MS = 8000
/** 同一个请求最多让用户手动重试几次。 */
export const NETWORK_RETRY_MANUAL_LIMIT = 3

/** 是否值得自动重试：只有 GET、没收到任何响应时才重试，POST/PUT/DELETE 可能已经执行过，不自动重发。 */
export function shouldAutoRetry(method: string | undefined, noResponse: boolean): boolean {
  return noResponse && (method || 'GET').toUpperCase() === 'GET'
}

/** 弹出带「重试」的提示。点了返回 true，关闭或超时返回 false。 */
export function askNetworkRetry(text: string, retryLabel: string): Promise<boolean> {
  return new Promise((resolve) => {
    let settled = false
    const settle = (value: boolean) => {
      if (settled) return
      settled = true
      resolve(value)
    }
    const handle = ElMessage({
      type: 'error',
      duration: NETWORK_RETRY_PROMPT_MS,
      showClose: true,
      grouping: true,
      onClose: () => settle(false),
      message: h('span', { class: 'network-retry-message' }, [
        h('span', text),
        h(
          'button',
          {
            type: 'button',
            class: 'network-retry-button',
            style:
              'margin-left:12px;padding:0;border:none;background:none;color:var(--el-color-primary);cursor:pointer;font-size:inherit',
            onClick: () => {
              settle(true)
              handle.close()
            }
          },
          retryLabel
        )
      ])
    })
  })
}
