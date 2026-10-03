/**
 * HTTP 请求封装模块
 * 基于 Axios 封装的 HTTP 请求工具，提供统一的请求/响应处理
 *
 * ## 主要功能
 *
 * - 请求/响应拦截器（自动添加 Token、统一错误处理）
 * - 401 未授权自动登出（带防抖机制）
 * - GET 请求没收到响应时自动重试一次，再不行弹带「重试」的提示
 * - 在线更新重启期间连不上后端不弹错误（只显示「正在更新」卡片）
 * - 统一的成功/错误消息提示
 * - 支持 GET/POST/PUT/DELETE 等常用方法
 *
 * @module utils/http
 * @author Art Design Pro Team
 */

import axios, { AxiosRequestConfig, AxiosResponse, InternalAxiosRequestConfig } from 'axios'
import { useUserStore } from '@/store/modules/user'
import { ApiStatus } from './status'
import { HttpError, handleError, noteQuietUpdateFailure, showError, showSuccess } from './error'
import {
  BACKEND_UNAVAILABLE_PAGE,
  backendUnreachableTracker,
  isBackendUnreachableFailure,
  onlineUpdateWindow,
  quietDuringOnlineUpdate,
  rememberBackendUnavailableReturn,
  shouldRedirectToBackendUnavailable
} from './backend-unavailable'
import { askNetworkRetry, NETWORK_RETRY_MANUAL_LIMIT, shouldAutoRetry } from './network-retry'
import { $t } from '@/locales'
import { BaseResponse } from '@/types'
import { buyerRebindMessage, noteStoreRebind, notifyCommercialRequired } from '@/utils/commercial'

/** 请求配置常量 */
const REQUEST_TIMEOUT = 15000
const LOGOUT_DELAY = 500
/** GET 没收到响应时，隔这么久自动再发一次。 */
const RETRY_DELAY = 1000
const UNAUTHORIZED_DEBOUNCE_TIME = 3000

/** 401防抖状态 */
let isUnauthorizedErrorShown = false
let unauthorizedTimer: NodeJS.Timeout | null = null

/** 扩展 AxiosRequestConfig */
interface ExtendedAxiosRequestConfig extends AxiosRequestConfig {
  showErrorMessage?: boolean
  showSuccessMessage?: boolean
}

const { VITE_API_URL, VITE_WITH_CREDENTIALS } = import.meta.env

/** Axios实例 */
const axiosInstance = axios.create({
  timeout: REQUEST_TIMEOUT,
  baseURL: VITE_API_URL,
  withCredentials: VITE_WITH_CREDENTIALS === 'true',
  validateStatus: (status) => status >= 200 && status < 300,
  transformResponse: [
    (data, headers) => {
      const contentType = headers['content-type']
      if (contentType?.includes('application/json')) {
        try {
          return JSON.parse(data)
        } catch {
          return data
        }
      }
      return data
    }
  ]
})

/** 请求拦截器 */
axiosInstance.interceptors.request.use(
  (request: InternalAxiosRequestConfig) => {
    const { accessToken } = useUserStore()
    if (accessToken) request.headers.set('Authorization', `Bearer ${accessToken}`)

    if (request.data && !(request.data instanceof FormData) && !request.headers['Content-Type']) {
      request.headers.set('Content-Type', 'application/json')
      request.data = JSON.stringify(request.data)
    }

    return request
  },
  (error) => {
    showError(createHttpError($t('httpMsg.requestConfigError'), ApiStatus.error))
    return Promise.reject(error)
  }
)

/** 响应拦截器 */
function noteBackendReachable(): void {
  backendUnreachableTracker.record(false)
}

function noteBackendUnreachable(
  status: number | undefined,
  hasResponse: boolean,
  code?: string
): void {
  if (code === 'ERR_CANCELED') return
  const tripped = backendUnreachableTracker.record(isBackendUnreachableFailure(status, hasResponse))
  if (!tripped || typeof window === 'undefined') return
  if (!shouldRedirectToBackendUnavailable(window.location.pathname)) return
  rememberBackendUnavailableReturn(window.sessionStorage, window.location)
  window.location.assign(BACKEND_UNAVAILABLE_PAGE)
}

axiosInstance.interceptors.response.use(
  async (response: AxiosResponse<BaseResponse>) => {
    noteBackendReachable()
    if (response.config.responseType === 'blob') {
      if (response.headers['content-disposition']?.includes('attachment')) return response
      const data = response.data as unknown as Blob
      try {
        response.data = JSON.parse(await data.text())
      } catch {
        throw createHttpError($t('httpMsg.requestFailed'), ApiStatus.error)
      }
    }
    const { code, msg, message } = response.data
    const responseMessage = msg || message
    if (code === 402) {
      notifyCommercialRequired(response.data)
      throw createHttpError(responseMessage || '该功能需要商业版', code)
    }
    if (noteStoreRebind(response.data?.data)) {
      throw createHttpError(responseMessage || buyerRebindMessage, code, response.data?.data)
    }
    if (code === ApiStatus.success) return response
    if (code === ApiStatus.unauthorized) handleUnauthorizedError(responseMessage)
    throw createHttpError(responseMessage || $t('httpMsg.requestFailed'), code, response.data?.data)
  },
  (error) => {
    noteBackendUnreachable(error.response?.status, Boolean(error.response), error.code)
    if (error.response?.status === ApiStatus.unauthorized) handleUnauthorizedError()
    return Promise.reject(handleError(error))
  }
)

/** 统一创建HttpError */
function createHttpError(message: string, code: number, data?: unknown) {
  return new HttpError(message, code, data === undefined ? undefined : { data })
}

/** 处理401错误（带防抖） */
function handleUnauthorizedError(message?: string): never {
  const error = createHttpError(message || $t('httpMsg.unauthorized'), ApiStatus.unauthorized)

  if (!isUnauthorizedErrorShown) {
    isUnauthorizedErrorShown = true
    logOut()

    unauthorizedTimer = setTimeout(resetUnauthorizedError, UNAUTHORIZED_DEBOUNCE_TIME)

    showError(error, true)
    throw error
  }

  throw error
}

/** 重置401防抖状态 */
function resetUnauthorizedError() {
  isUnauthorizedErrorShown = false
  if (unauthorizedTimer) clearTimeout(unauthorizedTimer)
  unauthorizedTimer = null
}

/** 退出登录函数 */
function logOut() {
  setTimeout(() => {
    useUserStore().logOut()
  }, LOGOUT_DELAY)
}

/** 延迟函数 */
function delay(ms: number) {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

/** 没收到任何响应的 GET 请求错误（只有这种才自动重试、才给「重试」按钮）。 */
function retryableFailure(config: ExtendedAxiosRequestConfig, error: unknown): error is HttpError {
  return error instanceof HttpError && shouldAutoRetry(config.method, error.noResponse)
}

/**
 * 发请求并处理失败重试：
 * GET 没收到响应时，先不提示，隔 1 秒自动再发一次；还不行就弹「暂时连不上服务器」+「重试」。
 * 点「重试」就再发，调用方拿到的是重试成功的结果；关掉提示才算失败。
 * POST/PUT/DELETE 可能已经在服务器上执行过，不自动重发，失败直接提示。
 * 在线更新重启期间不重试也不提示，交给「正在更新」卡片。
 */
async function requestWithRetry<T>(config: ExtendedAxiosRequestConfig): Promise<T> {
  try {
    return await request<T>(config, true)
  } catch (error) {
    if (!retryableFailure(config, error) || error.displayed) throw error
  }
  await delay(RETRY_DELAY)
  for (let manual = 0; ; manual++) {
    try {
      return await request<T>(config, true)
    } catch (error) {
      if (!retryableFailure(config, error) || error.displayed) throw error
      if (config.showErrorMessage === false) throw error
      if (manual >= NETWORK_RETRY_MANUAL_LIMIT) {
        showError(error, true)
        throw error
      }
      console.error('[HTTP Error]', error.toLogData())
      error.displayed = true
      const again = await askNetworkRetry(error.message, $t('httpMsg.retry'))
      if (!again || onlineUpdateWindow() !== null) throw error
    }
  }
}

/**
 * 请求函数
 * @param holdNetworkError 为 true 时，GET 没收到响应先不提示，由 requestWithRetry 决定重试还是提示
 */
async function request<T = any>(
  config: ExtendedAxiosRequestConfig,
  holdNetworkError = false
): Promise<T> {
  // POST | PUT 参数自动填充
  if (
    ['POST', 'PUT'].includes(config.method?.toUpperCase() || '') &&
    config.params &&
    !config.data
  ) {
    config.data = config.params
    config.params = undefined
  }

  try {
    const res = await axiosInstance.request<BaseResponse<T>>(config)
    if (config.responseType === 'blob') return res.data as unknown as T

    // 显示成功消息
    if (config.showSuccessMessage && res.data.msg) {
      showSuccess(res.data.msg)
    }

    return res.data.data as T
  } catch (error) {
    if (error instanceof HttpError && error.code !== ApiStatus.unauthorized && error.code !== 402) {
      if (quietDuringOnlineUpdate(error.status, !error.noResponse)) {
        noteQuietUpdateFailure(error)
      } else if (!(holdNetworkError && retryableFailure(config, error))) {
        showError(error, config.showErrorMessage !== false)
      }
    }
    return Promise.reject(error)
  }
}

/** API方法集合 */
const api = {
  get<T>(config: ExtendedAxiosRequestConfig) {
    return requestWithRetry<T>({ ...config, method: 'GET' })
  },
  post<T>(config: ExtendedAxiosRequestConfig) {
    return requestWithRetry<T>({ ...config, method: 'POST' })
  },
  put<T>(config: ExtendedAxiosRequestConfig) {
    return requestWithRetry<T>({ ...config, method: 'PUT' })
  },
  del<T>(config: ExtendedAxiosRequestConfig) {
    return requestWithRetry<T>({ ...config, method: 'DELETE' })
  },
  request<T>(config: ExtendedAxiosRequestConfig) {
    return requestWithRetry<T>(config)
  }
}

export default api
