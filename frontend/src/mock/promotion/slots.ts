import type { PromotionPage, PromotionSlot } from '@/api/promotion'

/**
 * 广告位投放数据（临时 mock，接入软件源后替换为 fetchPromotions(slot)）
 * 每页的 layout 由数据方决定：grid 为多格网格，banner 为整幅通栏
 */
export const promotionSlotMocks: Record<PromotionSlot, PromotionPage[]> = {
  dashboard: [
    {
      id: 'dashboard-extensions',
      layout: 'grid',
      items: [
        {
          id: 'plugin-market',
          tag: '推荐',
          title: '官方插件市场',
          summary: '一键安装官方与第三方扩展',
          linkUrl: '/plugin-store',
          accent: '#4f7cff'
        },
        {
          id: 'anti-piracy-pro',
          tag: '热门',
          title: '防盗版增强',
          summary: '设备指纹追踪与自动封禁',
          linkUrl: '',
          accent: '#ff6b5b'
        },
        {
          id: 'home-template-pack',
          tag: '新品',
          title: '门户模板包',
          summary: '十二套可直接启用的门户皮肤',
          linkUrl: '/home-template',
          accent: '#7c5cff'
        },
        {
          id: 'sms-notify',
          tag: '增值',
          title: '短信通知服务',
          summary: '到期提醒与异地登录预警',
          linkUrl: '',
          accent: '#00b3a4'
        },
        {
          id: 'realname-api',
          tag: '合规',
          title: '实名认证接口',
          summary: '三要素核验，按次计费',
          linkUrl: '',
          accent: '#2fa8ff'
        },
        {
          id: 'agent-distribution',
          tag: '推荐',
          title: '分销进阶版',
          summary: '多级返佣与阶梯定价策略',
          linkUrl: '',
          accent: '#ff9f2e'
        },
        {
          id: 'data-screen',
          tag: '新品',
          title: '数据大屏',
          summary: '经营数据可视化投屏组件',
          linkUrl: '',
          accent: '#17c0eb'
        },
        {
          id: 'i18n-pack',
          tag: '免费',
          title: '多语言包',
          summary: '内置英日韩三语翻译词条',
          linkUrl: '',
          accent: '#35c759'
        },
        {
          id: 'premium-support',
          tag: '服务',
          title: '专属技术支持',
          summary: '工作日两小时响应承诺',
          linkUrl: '',
          accent: '#f45d8c'
        }
      ]
    },
    {
      id: 'dashboard-enterprise',
      layout: 'banner',
      items: [
        {
          id: 'enterprise-upgrade',
          tag: '限时',
          title: '企业版年度特惠',
          summary: '不限授权数量、独立部署与优先技术支持，年付立减四成',
          linkUrl: '',
          accent: '#3d5afe'
        }
      ]
    }
  ],
  'plugin-store': [
    {
      id: 'store-featured',
      layout: 'grid',
      items: [
        {
          id: 'plugin-pass',
          tag: '年卡',
          title: '插件通行证',
          summary: '全站付费插件不限次下载与升级',
          linkUrl: 'https://gitee.com/Zcy-sa/auth-pro',
          accent: '#3d5afe'
        },
        {
          id: 'payment-aggregate',
          tag: '热门',
          title: '聚合支付插件',
          summary: '微信、支付宝、易支付一站接入',
          linkUrl: '',
          accent: '#00b3a4'
        },
        {
          id: 'risk-control',
          tag: '新品',
          title: '风控规则引擎',
          summary: '异常下单与批量解绑自动拦截',
          linkUrl: '/piracy',
          accent: '#ff6b5b'
        },
        {
          id: 'source-mirror',
          tag: '加速',
          title: '软件源加速节点',
          summary: '国内多线镜像，下载提速数倍',
          linkUrl: '/online-update',
          accent: '#7c5cff'
        },
        {
          id: 'device-fingerprint',
          tag: '防盗版',
          title: '设备指纹增强',
          summary: '硬件特征采集与多开识别',
          linkUrl: '/piracy',
          accent: '#ff9f2e'
        },
        {
          id: 'invoice-center',
          tag: '合规',
          title: '电子发票中心',
          summary: '订单开票与税务信息归档',
          linkUrl: '/order-list',
          accent: '#2fa8ff'
        },
        {
          id: 'card-batch',
          tag: '效率',
          title: '卡密批量工具',
          summary: '批量生成、导出与失效回收',
          linkUrl: '/license',
          accent: '#17c0eb'
        },
        {
          id: 'webhook-relay',
          tag: '集成',
          title: 'Webhook 转发',
          summary: '订单事件推送到企业微信与钉钉',
          linkUrl: '',
          accent: '#35c759'
        }
      ]
    }
  ]
}

export function fetchPromotionMocks(slot: PromotionSlot): Promise<PromotionPage[]> {
  return Promise.resolve(promotionSlotMocks[slot])
}
