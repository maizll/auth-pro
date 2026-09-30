// 短确认走 AppDialog 的 sm 档。调用方式和原来的确认框一样：取消会拒绝。
import { reactive } from 'vue'

type Settler = {
  resolve: (value: string | boolean) => void
  reject: (reason?: unknown) => void
}

export const appConfirmState = reactive({
  open: false,
  title: '确认',
  message: '',
  value: '',
  prompt: false,
  danger: false,
  confirmText: '确定',
  cancelText: '取消'
})

let pending: Settler | null = null

function open(
  message: string,
  title: string,
  options: Record<string, unknown> | undefined,
  prompt: boolean
) {
  appConfirmState.message = message
  appConfirmState.title = title || '确认'
  appConfirmState.prompt = prompt
  appConfirmState.value = ''
  appConfirmState.danger =
    options?.type === 'error' || options?.type === 'warning' || String(title).includes('危险')
  appConfirmState.confirmText = String(
    options?.confirmButtonText || (appConfirmState.danger ? '确定' : '确定')
  )
  appConfirmState.cancelText = String(options?.cancelButtonText || '取消')
  appConfirmState.open = true
  return new Promise<string | boolean>((resolve, reject) => {
    pending = { resolve, reject }
  })
}

export function appConfirm(message: string, title = '确认', options?: Record<string, unknown>) {
  return open(message, title, options, false)
}

export function appPrompt(message: string, title = '请填写', options?: Record<string, unknown>) {
  return open(message, title, options, true).then((value) => ({ value: String(value) }))
}

export function settleAppConfirm(ok: boolean) {
  const current = pending
  pending = null
  appConfirmState.open = false
  if (!current) return
  if (!ok) {
    current.reject(new Error('cancel'))
    return
  }
  current.resolve(appConfirmState.prompt ? appConfirmState.value : true)
}
