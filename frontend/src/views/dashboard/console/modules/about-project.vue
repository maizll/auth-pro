<template>
  <div class="art-card p-5 flex-b mb-5 max-sm:mb-4">
    <div>
      <h2 class="text-2xl font-medium">关于项目</h2>
      <p class="text-g-700 mt-1">{{ systemName }} 提供授权、代理、用户与校验日志管理</p>
      <p class="text-g-700 mt-1">文档与工单在本系统内，代码仓库仅指向本项目</p>

      <div class="flex flex-wrap gap-3.5 max-w-150 mt-9">
        <div
          class="w-60 flex-cb h-12.5 px-3.5 border border-g-300 c-p rounded-lg text-sm bg-g-100 duration-300 hover:-translate-y-1 max-sm:w-full"
          v-for="link in linkList"
          :key="link.label"
          @click="goPage(link)"
        >
          <span class="text-g-700">{{ link.label }}</span>
          <ArtSvgIcon icon="ri:arrow-right-s-line" class="text-lg text-g-600" />
        </div>
      </div>
    </div>
    <img class="w-75 max-md:!hidden" src="@imgs/draw/draw1.png" alt="draw1" />
  </div>
</template>

<script setup lang="ts">
  import { useSystemConfigStore } from '@/store/modules/system-config'
  import { WEB_LINKS } from '@/utils/constants'

  const router = useRouter()
  const { siteName: systemName } = storeToRefs(useSystemConfigStore())

  const linkList = [
    { label: '开发文档', routeName: 'DeveloperDoc' },
    { label: '工单', routeName: 'TicketManage' },
    { label: '代码仓库', url: WEB_LINKS.GITHUB }
  ]

  /**
   * 站内文档走路由；仅本仓库地址新开标签。
   */
  const goPage = (link: { routeName?: string; url?: string }): void => {
    if (link.routeName) {
      void router.push({ name: link.routeName })
      return
    }
    if (link.url) {
      window.open(link.url, '_blank', 'noopener,noreferrer')
    }
  }
</script>
