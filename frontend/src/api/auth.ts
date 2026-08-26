import request from '@/utils/http'

/**
 * 登录
 * @param params 登录参数
 * @returns 登录响应
 */
export function fetchLogin(params: Api.Auth.LoginParams) {
  return request.post<Api.Auth.LoginResponse>({
    url: '/api/auth/login',
    params
    // showSuccessMessage: true // 显示成功消息
    // showErrorMessage: false // 不显示错误消息
  })
}

/**
 * 获取用户信息
 * @returns 用户信息
 */
export function fetchGetUserInfo() {
  return request.get<Api.Auth.UserInfo>({
    url: '/api/user/info'
    // 自定义请求头
    // headers: {
    //   'X-Custom-Header': 'your-custom-value'
    // }
  })
}

/**
 * 更新当前管理员资料
 * @param params 昵称 / 邮箱 / 头像，只传需要修改的字段
 */
export function fetchUpdateUserInfo(params: Api.Auth.UpdateUserInfoParams) {
  return request.put<null>({
    url: '/api/user/info',
    params
  })
}

/**
 * 修改当前管理员密码
 */
export function fetchChangePassword(params: Api.Auth.ChangePasswordParams) {
  return request.post<null>({
    url: '/api/user/change-password',
    params
  })
}
