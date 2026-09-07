// 极验行为验证 4.0 前端封装（bind 模式：提交时弹出滑块，成功后随请求提交四要素）
// 文档：https://docs.geetest.com/gt4/apirefer/api/web
import { computed, onBeforeUnmount } from 'vue'
import { useSystemConfigStore } from '@/store/modules/system-config'

/** 前端验证成功后得到的四要素，随登录请求提交给服务端二次校验 */
export interface GeetestValidateResult {
  lot_number: string
  captcha_output: string
  pass_token: string
  gen_time: string
}

interface GeetestCaptchaObj {
  onReady(cb: () => void): GeetestCaptchaObj
  onSuccess(cb: () => void): GeetestCaptchaObj
  onClose(cb: () => void): GeetestCaptchaObj
  onError(cb: (err: unknown) => void): GeetestCaptchaObj
  showCaptcha(): void
  getValidate(): GeetestValidateResult | false
  reset(): void
  destroy(): void
}

declare global {
  interface Window {
    initGeetest4?: (
      config: Record<string, unknown>,
      handler: (captcha: GeetestCaptchaObj) => void
    ) => void
  }
}

const GT4_SCRIPT_URL = 'https://static.geetest.com/v4/gt4.js'
let gt4Loader: Promise<void> | null = null

/** 加载 gt4.js（全局一次，失败后可重试） */
export function loadGeetest4(): Promise<void> {
  if (window.initGeetest4) return Promise.resolve()
  if (gt4Loader) return gt4Loader
  gt4Loader = new Promise<void>((resolve, reject) => {
    const script = document.createElement('script')
    script.src = GT4_SCRIPT_URL
    script.async = true
    script.onload = () => resolve()
    script.onerror = () => {
      gt4Loader = null
      reject(new Error('行为验证组件加载失败，请检查网络后刷新页面'))
    }
    document.head.appendChild(script)
  })
  return gt4Loader
}

interface GeetestCaptchaController {
  /** 弹出滑块并等待结果；用户关闭或出错时 resolve(null) */
  verify(): Promise<GeetestValidateResult | null>
  reset(): void
  destroy(): void
}

function createGeetestCaptcha(captchaId: string): Promise<GeetestCaptchaController> {
  return new Promise((resolve, reject) => {
    window.initGeetest4!(
      {
        captchaId,
        product: 'bind',
        language: 'zho',
        onError: (err: unknown) =>
          reject(err instanceof Error ? err : new Error('行为验证初始化失败'))
      },
      (captcha) => {
        let pending: ((result: GeetestValidateResult | null) => void) | null = null
        const settle = (result: GeetestValidateResult | null) => {
          pending?.(result)
          pending = null
        }
        captcha
          .onReady(() =>
            resolve({
              verify: () =>
                new Promise<GeetestValidateResult | null>((res) => {
                  pending = res
                  captcha.showCaptcha()
                }),
              reset: () => captcha.reset(),
              destroy: () => captcha.destroy()
            })
          )
          .onSuccess(() => settle(captcha.getValidate() || null))
          .onClose(() => settle(null))
          .onError(() => settle(null))
      }
    )
  })
}

/**
 * 登录页共用的极验验证入口。
 * - captchaEnabled：系统配置开启且已配置验证 ID 时为 true
 * - verify：返回 undefined 表示未启用（直接提交）；返回 null 表示用户取消或组件出错（应中止提交）
 * - reset：登录失败（如密码错误）后重置验证状态
 */
export function useGeetestLoginCaptcha() {
  const systemConfigStore = useSystemConfigStore()
  const captchaEnabled = computed(
    () => systemConfigStore.captchaEnabled && !!systemConfigStore.captchaId
  )

  let controller: GeetestCaptchaController | null = null
  let initializing: Promise<GeetestCaptchaController> | null = null

  async function ensureController(): Promise<GeetestCaptchaController> {
    if (controller) return controller
    if (initializing) return initializing
    initializing = (async () => {
      await loadGeetest4()
      const instance = await createGeetestCaptcha(systemConfigStore.captchaId)
      controller = instance
      return instance
    })()
    try {
      return await initializing
    } finally {
      initializing = null
    }
  }

  /** 提前加载并初始化验证组件，减少点击登录时的等待 */
  async function preload() {
    if (!systemConfigStore.loaded) {
      await systemConfigStore.loadPublicConfig()
    }
    if (!captchaEnabled.value) return
    try {
      await ensureController()
    } catch {
      /* 预加载失败不阻塞页面，提交时再提示 */
    }
  }

  async function verify(): Promise<GeetestValidateResult | null | undefined> {
    if (!systemConfigStore.loaded) {
      await systemConfigStore.loadPublicConfig()
    }
    if (!captchaEnabled.value) return undefined
    const instance = await ensureController()
    return instance.verify()
  }

  function reset() {
    controller?.reset()
  }

  onBeforeUnmount(() => {
    controller?.destroy()
    controller = null
  })

  return { captchaEnabled, preload, verify, reset }
}
