<!-- 应用编辑弹框里的「商业版」小节。只在官网显示：每个应用各自出售，套餐、订单、授权、绑定互不相通。 -->
<template>
  <div class="app-commercial">
    <div class="app-commercial__title">商业版</div>
    <ElFormItem label="出售商业版">
      <div class="app-commercial__row">
        <ElSwitch
          :model-value="form.mode === 'selling'"
          :before-change="beforeToggle"
          aria-label="出售商业版"
        />
        <ElTag :type="statusTag.type" size="small">{{ statusTag.label }}</ElTag>
        <ElButton
          v-for="gap in gaps"
          :key="gap.code"
          link
          type="danger"
          size="small"
          @click="emit('gap', gap)"
        >
          {{ gap.label }}
        </ElButton>
      </div>
      <div class="form-tip">
        <template v-if="form.mode === 'stopped'">
          已停止新售，已售出的商业版照常使用。打开开关可以恢复出售。
          <ElButton v-if="canTurnPlain" link type="primary" size="small" @click="form.mode = 'off'">
            转为普通应用
          </ElButton>
        </template>
        <template v-else>每个应用各自独立：套餐、订单、授权、绑定互不相通。</template>
      </div>
    </ElFormItem>

    <template v-if="form.mode !== 'off'">
      <ElFormItem class="app-commercial__box-item">
        <div class="app-commercial__box">
          <div class="app-commercial__box-head">
            <b>本应用的商业版套餐</b>
            <ElButton link type="primary" :disabled="!appId" @click="emit('plans')">
              管理套餐
            </ElButton>
          </div>
          <table v-if="plans.length" class="app-commercial__plans">
            <thead>
              <tr><th>套餐</th><th>时长</th><th>价格</th><th>状态</th></tr>
            </thead>
            <tbody>
              <tr v-for="plan in plans" :key="plan.id">
                <td>{{ plan.name }}</td>
                <td>{{ plan.durationDays > 0 ? `${plan.durationDays} 天` : '永久' }}</td>
                <td>¥{{ plan.price }}</td>
                <td>
                  <ElTag size="small" :type="plan.enabled ? 'success' : 'info'">
                    {{ plan.enabled ? '上架' : '下架' }}
                  </ElTag>
                </td>
              </tr>
            </tbody>
          </table>
          <div v-else class="app-commercial__empty">
            {{
              appId
                ? '还没有带价格的套餐。买家看不到购买入口，先去「管理套餐」添加。'
                : '保存应用后，在「套餐管理」里给它添加商业版套餐。'
            }}
          </div>
          <div v-if="stats" class="form-tip">
            有效授权 {{ stats.activeLicenses }} 个 · 已绑定站点 {{ stats.boundSites }} 个 ·
            待支付订单 {{ stats.pendingOrders }} 笔
          </div>
        </div>
      </ElFormItem>

      <ElFormItem label="老客户端">
        <div class="app-commercial__full">
          <ElCheckbox :model-value="form.legacyDefault" @change="onLegacyChange">
            接收未声明应用的老客户端（1.8.3 及更早）
          </ElCheckbox>
          <div class="form-tip">
            全站只能有一个应用勾选。老客户端绑定、刷新和购买都会落到这个应用。已绑定的站点一律按绑定时的应用处理，不受这里影响。
          </div>
        </div>
      </ElFormItem>

      <ElFormItem v-if="appKey" label="客户端标识">
        <div class="app-commercial__full">
          <ElInput :model-value="appKey" readonly>
            <template #append>
              <ElButton @click="copyKey">复制</ElButton>
            </template>
          </ElInput>
          <div class="form-tip"
            >新版客户端打包时写入这个标识，顶栏购买窗口就只显示本应用的套餐。</div
          >
        </div>
      </ElFormItem>

      <ElFormItem>
        <ElCollapse class="app-commercial__full">
          <ElCollapseItem title="高级设置（本应用单独生效）" name="advanced">
            <ElFormItem label="离线宽限天数">
              <ElInputNumber v-model="form.graceDays" :min="1" :max="30" />
              <div class="form-tip">源站暂时连不上时，买家商业版还能继续用的天数，默认 7 天。</div>
            </ElFormItem>
            <ElFormItem label="改密撤销绑定">
              <ElSwitch v-model="form.revokeOnPasswordChange" />
              <div class="form-tip">打开后，买家在源站改密码，会撤销他在本应用下绑定的站点。</div>
            </ElFormItem>
            <ElFormItem label="功能键">
              <ElInput v-model.trim="form.features" placeholder="一般不用改" />
              <div class="form-tip">商业版开放的能力，多个用英文逗号隔开。默认已填好多应用。</div>
            </ElFormItem>
          </ElCollapseItem>
        </ElCollapse>
      </ElFormItem>
    </template>
  </div>
</template>

<script setup lang="ts">
  import { computed } from 'vue'
  import { ElMessage } from 'element-plus'
  import { appConfirm } from '@/utils/app-confirm'
  import type {
    AppCommercialMode,
    AppCommercialPlan,
    AppCommercialStats,
    AppSaleGap
  } from '@/api/license-manage'

  defineOptions({ name: 'AppCommercialSection' })

  export interface AppCommercialForm {
    mode: AppCommercialMode
    legacyDefault: boolean
    graceDays: number
    revokeOnPasswordChange: boolean
    features: string
  }

  const form = defineModel<AppCommercialForm>('form', { required: true })

  const props = defineProps<{
    /** 保存前的状态，用来判断是关闭还是转为普通应用 */
    savedMode: AppCommercialMode
    appId: number
    appKey: string
    plans: AppCommercialPlan[]
    stats?: AppCommercialStats
    gaps: AppSaleGap[]
    /** 当前接收老客户端的其他应用名，没有就是空 */
    legacyOwner: string
  }>()

  const emit = defineEmits<{
    /** 已有有效授权或待支付订单时，关闭要走确认弹框 */
    (e: 'close'): void
    (e: 'plans'): void
    (e: 'gap', gap: AppSaleGap): void
  }>()

  const hasSales = computed(
    () => !!props.stats && (props.stats.activeLicenses > 0 || props.stats.pendingOrders > 0)
  )
  const canTurnPlain = computed(() => !hasSales.value)

  const statusTag = computed<{ type: 'success' | 'info' | 'danger' | 'primary'; label: string }>(
    () => {
      if (form.value.mode === 'stopped') return { type: 'info', label: '已停售' }
      if (form.value.mode === 'off') return { type: 'info', label: '未出售' }
      if (props.savedMode === 'selling' && props.gaps.length)
        return { type: 'danger', label: '缺项' }
      return { type: 'success', label: props.savedMode === 'selling' ? '出售中' : '保存后开始出售' }
    }
  )

  function beforeToggle() {
    if (form.value.mode !== 'selling') {
      form.value.mode = 'selling'
      return false
    }
    // 已经卖出去的应用，关闭要选「仅停售」还是「作废」，交给确认弹框。
    if (props.savedMode !== 'off' && hasSales.value) {
      emit('close')
      return false
    }
    form.value.mode = props.savedMode === 'stopped' ? 'stopped' : 'off'
    return false
  }

  async function onLegacyChange(value: string | number | boolean) {
    const checked = value === true
    if (checked && props.legacyOwner) {
      try {
        await appConfirm(
          `目前接收老客户端的是「${props.legacyOwner}」。改到当前应用后，老客户端以后的新绑定会落到这里。已绑定的站点不受影响。`,
          '改变接收老客户端的应用',
          { confirmButtonText: '改到当前应用', cancelButtonText: '取消' }
        )
      } catch {
        return
      }
    }
    form.value.legacyDefault = checked
  }

  async function copyKey() {
    try {
      await navigator.clipboard.writeText(props.appKey)
      ElMessage.success('已复制客户端标识')
    } catch {
      ElMessage.error('复制失败，请手动选中复制')
    }
  }
</script>

<style scoped lang="scss">
  .app-commercial {
    &__title {
      display: flex;
      gap: 8px;
      align-items: center;
      margin: 4px 0 12px;
      font-weight: 600;

      &::before {
        width: 3px;
        height: 14px;
        content: '';
        background: var(--el-color-primary);
        border-radius: 2px;
      }
    }

    &__row {
      display: flex;
      flex-wrap: wrap;
      gap: 8px;
      align-items: center;
    }

    &__full {
      width: 100%;
    }

    &__box {
      width: 100%;
      padding: 10px 12px;
      background: var(--el-fill-color-light);
      border: 1px solid var(--el-border-color-lighter);
      border-radius: 6px;
    }

    &__box-head {
      display: flex;
      align-items: center;
      justify-content: space-between;
    }

    &__plans {
      width: 100%;
      margin-top: 6px;
      font-size: 13px;
      border-collapse: collapse;

      th,
      td {
        padding: 6px 4px;
        text-align: left;
        border-bottom: 1px solid var(--el-border-color-lighter);
      }

      th {
        font-weight: 500;
        color: var(--el-text-color-secondary);
      }
    }

    &__empty {
      margin-top: 6px;
      font-size: 13px;
      color: var(--el-text-color-secondary);
    }

    .form-tip {
      width: 100%;
      margin-top: 4px;
      font-size: 12px;
      line-height: 1.5;
      color: var(--el-text-color-secondary);
    }
  }
</style>
