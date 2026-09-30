<!-- 用户菜单 -->
<template>
  <ElPopover
    ref="userMenuPopover"
    placement="bottom-end"
    :width="240"
    :hide-after="0"
    :offset="10"
    :trigger="narrow ? 'click' : 'hover'"
    :show-arrow="false"
    popper-class="user-menu-popover"
    popper-style="padding: 5px 16px;"
  >
    <template #reference>
      <img
        class="user-menu-avatar size-8.5 mr-5 c-p rounded-full"
        src="@imgs/user/avatar.webp"
        alt="avatar"
      />
    </template>
    <template #default>
      <div class="pt-3">
        <div class="flex-c pb-1 px-0">
          <img
            class="w-10 h-10 mr-3 ml-0 overflow-hidden rounded-full float-left"
            src="@imgs/user/avatar.webp"
          />
          <div class="w-[calc(100%-60px)] h-full">
            <span class="block text-sm font-medium text-g-800 truncate">{{
              userInfo.userName
            }}</span>
            <span class="block mt-0.5 text-xs text-g-500 truncate">{{ userInfo.email }}</span>
          </div>
        </div>
        <ul class="py-4 mt-3 border-t border-g-300/80">
          <li class="btn-item" @click="goPage('/system/user-center')">
            <ArtSvgIcon icon="ri:user-3-line" />
            <span>{{ $t('topBar.user.userCenter') }}</span>
          </li>
          <li
            v-if="showNarrowTools && shouldShowLanguage"
            class="btn-item"
            @click.stop="languageOpen = !languageOpen"
          >
            <ArtSvgIcon icon="ri:translate-2" />
            <span>切换语言</span>
          </li>
          <li
            v-for="item in narrowLanguageOptions"
            :key="item.value"
            class="btn-item lang-sub"
            @click="pickLanguage(item.value)"
          >
            <span>{{ item.label }}</span>
            <ArtSvgIcon v-if="locale === item.value" icon="ri:check-fill" class="ml-auto" />
          </li>
          <li v-if="showNarrowTools && shouldShowSettings" class="btn-item" @click="openSetting">
            <ArtSvgIcon icon="ri:settings-line" />
            <span>主题设置</span>
          </li>
          <li v-if="showNarrowTools && shouldShowThemeToggle" class="btn-item" @click="toggleTheme">
            <ArtSvgIcon :icon="isDark ? 'ri:sun-fill' : 'ri:moon-line'" />
            <span>暗色模式</span>
          </li>
          <div class="w-full h-px my-2 bg-g-300/80"></div>
          <div class="log-out c-p" @click="loginOut">
            {{ $t('topBar.user.logout') }}
          </div>
        </ul>
      </div>
    </template>
  </ElPopover>
</template>

<script setup lang="ts">
  import { appConfirm } from '@/utils/app-confirm'
  import { computed, ref } from 'vue'
  import { useI18n } from 'vue-i18n'
  import { useRouter } from 'vue-router'

  import { LanguageEnum } from '@/enums/appEnum'
  import { useHeaderBar } from '@/hooks/core/useHeaderBar'
  import { useNarrowScreen } from '@/hooks/core/useNarrowScreen'
  import { languageOptions } from '@/locales'
  import { useSettingStore } from '@/store/modules/setting'
  import { useUserStore } from '@/store/modules/user'
  import { mittBus } from '@/utils/sys'
  import { themeAnimation } from '@/utils/ui/animation'
  import { useCommon } from '@/hooks/core/useCommon'

  defineOptions({ name: 'ArtUserMenu' })

  const router = useRouter()
  const { t, locale } = useI18n()
  const userStore = useUserStore()
  const settingStore = useSettingStore()
  const narrow = useNarrowScreen(767)
  const { shouldShowLanguage, shouldShowSettings, shouldShowThemeToggle } = useHeaderBar()
  const { isDark, showSettingGuide } = storeToRefs(settingStore)
  const { refresh } = useCommon()

  const { getUserInfo: userInfo } = storeToRefs(userStore)
  const userMenuPopover = ref()
  const languageOpen = ref(false)
  const showNarrowTools = computed(() => narrow.value)
  const narrowLanguageOptions = computed(() =>
    showNarrowTools.value && shouldShowLanguage.value && languageOpen.value ? languageOptions : []
  )

  /**
   * 页面跳转
   * @param {string} path - 目标路径
   */
  const goPage = (path: string): void => {
    closeUserMenu()
    router.push(path)
  }

  const pickLanguage = (lang: LanguageEnum): void => {
    closeUserMenu()
    if (locale.value === lang) return
    locale.value = lang
    userStore.setLanguage(lang)
    setTimeout(() => refresh(), 50)
  }

  const openSetting = (): void => {
    closeUserMenu()
    mittBus.emit('openSetting')
    if (showSettingGuide.value) settingStore.hideSettingGuide()
  }

  const toggleTheme = (event: MouseEvent): void => {
    closeUserMenu()
    themeAnimation(event)
  }

  /**
   * 用户登出确认
   */
  const loginOut = (): void => {
    closeUserMenu()
    setTimeout(() => {
      appConfirm(t('common.logOutTips'), t('common.tips'), {
        confirmButtonText: t('common.confirm'),
        cancelButtonText: t('common.cancel'),
        customClass: 'login-out-dialog'
      }).then(() => {
        userStore.logOut()
      })
    }, 200)
  }

  /**
   * 关闭用户菜单弹出层
   */
  const closeUserMenu = (): void => {
    setTimeout(() => {
      userMenuPopover.value.hide()
    }, 100)
  }
</script>

<style scoped>
  @reference '@styles/core/tailwind.css';

  @layer components {
    .btn-item {
      @apply flex items-center p-2 mb-3 select-none rounded-md cursor-pointer last:mb-0;

      span {
        @apply text-sm;
      }

      .art-svg-icon {
        @apply mr-2 text-base;
      }

      &:hover {
        background-color: var(--art-gray-200);
      }
    }

    .lang-sub {
      min-height: 36px;
      padding-left: 34px;
    }
  }

  @media (max-width: 767px) {
    .user-menu-avatar {
      width: 36px !important;
      height: 36px !important;
      margin-right: 12px !important;
    }

    .btn-item,
    .log-out {
      min-height: 36px;
    }

    .log-out {
      display: flex;
      align-items: center;
      justify-content: center;
    }
  }

  .log-out {
    @apply py-1.5
    mt-5
    text-xs
    text-center
    border
    border-g-400
    rounded-md
    transition-all
    duration-200
    hover:shadow-xl;
  }
</style>
