<template>
  <div class="ads-promote-page">
    <el-card shadow="never" class="art-card mb-3">
      <div class="page-head">
        <div>
          <h2 class="title">推广投放</h2>
          <p class="hint">
            把网站推荐到其他站长后台。付款后自动检查；不过退回修改（不退款）。展示在其他站，不在本站。
          </p>
        </div>
      </div>
      <el-tabs v-model="tab">
        <el-tab-pane label="新建投放" name="new" />
        <el-tab-pane :label="`我的投放 (${orders.length})`" name="mine" />
      </el-tabs>
    </el-card>

    <template v-if="tab === 'new'">
      <div class="buy-grid">
        <el-card shadow="never" class="art-card">
          <div class="sec">推广地址</div>
          <el-input v-model="form.linkUrl" size="small" placeholder="https://..." maxlength="500" />
          <el-form label-position="top" class="mt-3">
            <el-form-item label="标题">
              <el-input v-model="form.title" size="small" maxlength="20" show-word-limit />
            </el-form-item>
            <el-form-item label="简介">
              <el-input
                v-model="form.description"
                size="small"
                type="textarea"
                :rows="2"
                maxlength="60"
                show-word-limit
              />
            </el-form-item>
            <el-form-item label="配图 URL（可选）">
              <el-input
                v-model="form.imageUrl"
                size="small"
                placeholder="https://... 或本站上传地址"
              />
            </el-form-item>
          </el-form>

          <div class="sec">广告位 <span class="muted">（订单/支付/设置页不售）</span></div>
          <div class="price-pick">
            <div
              v-for="s in slots"
              :key="s.id"
              class="pp"
              :class="{ on: form.slotId === s.id, dis: !s.enabled }"
              @click="s.enabled && (form.slotId = s.id) && loadCalendar()"
            >
              <el-radio
                :model-value="form.slotId"
                :label="s.id"
                :disabled="!s.enabled"
                size="small"
              />
              <div>
                <div class="pp__t">{{ s.name }}</div>
                <div class="pp__d">{{ s.description }}</div>
              </div>
              <div class="pp__p">
                ¥{{ (s.priceCents / 100).toFixed(0) }}/天
                <small>{{ s.enabled ? `余约 ${s.leftHint}` : '停售' }}</small>
              </div>
            </div>
          </div>

          <div class="sec">投放日期</div>
          <div class="cal-wrap">
            <div class="cal-head">
              <el-button size="small" text @click="shiftMonth(-1)">‹</el-button>
              <b>{{ monthLabel }}</b>
              <el-button size="small" text @click="shiftMonth(1)">›</el-button>
            </div>
            <div class="cal">
              <div
                v-for="d in calendarDays"
                :key="d.date"
                class="cal__d"
                :class="dayClass(d)"
                @click="toggleDay(d)"
              >
                {{ d.day }}
                <small v-if="d.full">已满</small>
                <small v-else-if="selectedDays.includes(d.date)">已选</small>
                <small v-else>¥{{ (d.priceCents / 100).toFixed(0) }}</small>
              </div>
            </div>
          </div>
        </el-card>

        <el-card shadow="never" class="art-card">
          <div class="sec" style="margin-top: 0">订单</div>
          <div class="order-line">
            <span>{{ slotName }} × {{ selectedDays.length }} 天</span>
            <span>¥{{ totalYuan }}</span>
          </div>
          <div class="nr-box">
            <b>虚拟产品，付款后不退款</b>
            <ul>
              <li>审核不过 → 回编辑，天数保留</li>
              <li>未展示天数 → 顺延</li>
              <li>违规下线 → 剩余天数作废</li>
            </ul>
          </div>
          <el-checkbox v-model="agree" size="small" class="mb-2">
            我已阅读并同意以上规则（虚拟产品不退款）
          </el-checkbox>
          <el-button
            type="primary"
            size="small"
            style="width: 100%"
            :disabled="!canCheckout"
            :loading="creating"
            @click="goCheckout"
          >
            去结账
          </el-button>
        </el-card>
      </div>
    </template>

    <el-card v-else shadow="never" class="art-card">
      <el-table :data="orders" size="small">
        <el-table-column prop="orderNo" label="订单号" min-width="160" />
        <el-table-column prop="slotName" label="广告位" width="120" />
        <el-table-column prop="title" label="标题" min-width="140" />
        <el-table-column prop="statusLabel" label="状态" width="120" />
        <el-table-column prop="amount" label="金额" width="90" />
        <el-table-column label="操作" width="120">
          <template #default="{ row }">
            <el-button
              v-if="row.status === 'need_edit'"
              size="small"
              type="primary"
              link
              @click="openEdit(row)"
            >
              修改重交
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <AppDialog v-model="checkoutOpen" title="确认订单并付款" size="md" flow="short">
      <div class="nr-box">
        <b>虚拟产品，付款后不退款</b>
        <p class="muted">{{ noRefundNotice }}</p>
      </div>
      <div class="order-line"
        ><span>应付</span><b>¥{{ pendingAmount }}</b></div
      >
      <div class="sec">支付方式</div>
      <el-radio-group v-model="payCode" size="small">
        <el-radio v-for="p in payOptions" :key="p.code" :label="p.code">{{ p.label }}</el-radio>
      </el-radio-group>
      <el-checkbox v-model="agreePay" size="small" class="mt-3 block">
        我已阅读并同意以上规则（虚拟产品不退款）
      </el-checkbox>
      <template #footer>
        <el-button size="small" @click="checkoutOpen = false">取消</el-button>
        <el-button
          type="primary"
          size="small"
          :disabled="!agreePay || !payCode"
          :loading="paying"
          @click="doPay"
        >
          付款 ¥{{ pendingAmount }}
        </el-button>
      </template>
    </AppDialog>

    <AppDialog v-model="editOpen" title="修改投放并重新提交" size="md" flow="short">
      <el-form label-position="top">
        <el-form-item label="标题">
          <el-input v-model="editForm.title" size="small" maxlength="20" show-word-limit />
        </el-form-item>
        <el-form-item label="简介">
          <el-input
            v-model="editForm.description"
            size="small"
            type="textarea"
            :rows="2"
            maxlength="60"
          />
        </el-form-item>
        <el-form-item label="链接">
          <el-input v-model="editForm.linkUrl" size="small" />
        </el-form-item>
        <el-form-item label="配图">
          <el-input v-model="editForm.imageUrl" size="small" />
        </el-form-item>
      </el-form>
      <p class="muted">不退款，已购天数保留。</p>
      <template #footer>
        <el-button size="small" @click="editOpen = false">取消</el-button>
        <el-button type="primary" size="small" :loading="resubmitting" @click="doResubmit"
          >重新提交</el-button
        >
      </template>
    </AppDialog>
  </div>
</template>

<script setup lang="ts">
  import { computed, onMounted, reactive, ref } from 'vue'
  import { ElMessage } from 'element-plus'
  import AppDialog from '@/components/core/dialog/AppDialog.vue'
  import {
    AD_NO_REFUND_NOTICE,
    createAdOrder,
    fetchAdCalendar,
    fetchAdPayOptions,
    fetchAdSlotCatalog,
    fetchMyAdOrders,
    payAdOrder,
    resubmitAdOrder
  } from '@/api/advertisement'

  defineOptions({ name: 'AdsPromote' })

  const tab = ref('new')
  const slots = ref<Array<any>>([])
  const noRefundNotice = ref(AD_NO_REFUND_NOTICE)
  const form = reactive({
    slotId: '',
    title: '',
    description: '',
    imageUrl: '',
    linkUrl: ''
  })
  const month = ref(new Date().toISOString().slice(0, 7))
  const calendarDays = ref<Array<any>>([])
  const selectedDays = ref<string[]>([])
  const agree = ref(false)
  const creating = ref(false)
  const checkoutOpen = ref(false)
  const pendingOrderNo = ref('')
  const pendingAmount = ref('0.00')
  const payOptions = ref<Array<{ code: string; label: string }>>([])
  const payCode = ref('')
  const agreePay = ref(false)
  const paying = ref(false)
  const orders = ref<Array<any>>([])
  const editOpen = ref(false)
  const editForm = reactive({ orderNo: '', title: '', description: '', linkUrl: '', imageUrl: '' })
  const resubmitting = ref(false)

  const monthLabel = computed(() => month.value.replace('-', ' 年 ') + ' 月')
  const slotName = computed(() => slots.value.find((s) => s.id === form.slotId)?.name || '未选')
  const totalYuan = computed(() => {
    const slot = slots.value.find((s) => s.id === form.slotId)
    if (!slot) return '0'
    return ((slot.priceCents * selectedDays.value.length) / 100).toFixed(0)
  })
  const canCheckout = computed(
    () =>
      agree.value &&
      !!form.slotId &&
      !!form.title.trim() &&
      !!form.linkUrl.trim() &&
      selectedDays.value.length > 0
  )

  function dayClass(d: any) {
    return {
      past: d.past,
      full: d.full,
      sel: selectedDays.value.includes(d.date)
    }
  }

  function toggleDay(d: any) {
    if (d.full || d.past) return
    const i = selectedDays.value.indexOf(d.date)
    if (i >= 0) selectedDays.value.splice(i, 1)
    else selectedDays.value.push(d.date)
    selectedDays.value.sort()
  }

  function shiftMonth(delta: number) {
    const [y, m] = month.value.split('-').map(Number)
    const dt = new Date(y, m - 1 + delta, 1)
    month.value = `${dt.getFullYear()}-${String(dt.getMonth() + 1).padStart(2, '0')}`
    loadCalendar()
  }

  async function loadSlots() {
    const data = await fetchAdSlotCatalog()
    slots.value = data?.list || []
    noRefundNotice.value = data?.noRefundNotice || AD_NO_REFUND_NOTICE
    if (!form.slotId && slots.value.length) {
      const first = slots.value.find((s) => s.enabled) || slots.value[0]
      form.slotId = first.id
      await loadCalendar()
    }
  }

  async function loadCalendar() {
    if (!form.slotId) return
    const data = await fetchAdCalendar(form.slotId, month.value)
    const today = new Date().toISOString().slice(0, 10)
    calendarDays.value = (data?.days || []).map((d: any) => ({
      ...d,
      day: Number(d.date.slice(-2)),
      past: d.date < today
    }))
  }

  async function loadOrders() {
    const data = await fetchMyAdOrders()
    orders.value = data?.list || []
  }

  async function goCheckout() {
    creating.value = true
    try {
      const data: any = await createAdOrder({
        slotId: form.slotId,
        title: form.title,
        description: form.description,
        imageUrl: form.imageUrl,
        linkUrl: form.linkUrl,
        days: selectedDays.value,
        agree: true
      })
      pendingOrderNo.value = data.orderNo
      pendingAmount.value = data.amount
      const opts = await fetchAdPayOptions()
      payOptions.value = opts?.list || []
      payCode.value = payOptions.value[0]?.code || ''
      agreePay.value = false
      checkoutOpen.value = true
    } catch (e: any) {
      ElMessage.error(e?.message || '创建订单失败')
    } finally {
      creating.value = false
    }
  }

  async function doPay() {
    paying.value = true
    try {
      const data: any = await payAdOrder({
        orderNo: pendingOrderNo.value,
        payCode: payCode.value,
        agree: true
      })
      if (data?.payUrl) {
        window.location.href = data.payUrl
        return
      }
      ElMessage.success(data?.msg || '请完成支付')
      checkoutOpen.value = false
      tab.value = 'mine'
      await loadOrders()
    } catch (e: any) {
      ElMessage.error(e?.message || '付款失败')
    } finally {
      paying.value = false
    }
  }

  function openEdit(row: any) {
    editForm.orderNo = row.orderNo
    editForm.title = row.title || ''
    editForm.description = ''
    editForm.linkUrl = ''
    editForm.imageUrl = ''
    editOpen.value = true
  }

  async function doResubmit() {
    resubmitting.value = true
    try {
      await resubmitAdOrder({ ...editForm })
      ElMessage.success('已重新提交（不退款，天数保留）')
      editOpen.value = false
      await loadOrders()
    } catch (e: any) {
      ElMessage.error(e?.message || '提交失败')
    } finally {
      resubmitting.value = false
    }
  }

  onMounted(async () => {
    await loadSlots()
    await loadOrders()
  })
</script>

<style scoped lang="scss">
  .title {
    margin: 0;
    font-size: 16px;
    font-weight: 600;
  }
  .hint {
    margin: 4px 0 0;
    font-size: 12px;
    color: #8a919f;
    line-height: 1.55;
  }
  .buy-grid {
    display: grid;
    grid-template-columns: minmax(0, 1.2fr) minmax(280px, 0.8fr);
    gap: 14px;
  }
  .sec {
    margin: 12px 0 8px;
    font-size: 13px;
    font-weight: 600;
  }
  .muted {
    font-weight: 400;
    font-size: 12px;
    color: #8a919f;
  }
  .price-pick {
    display: grid;
    gap: 6px;
  }
  .pp {
    display: flex;
    gap: 8px;
    align-items: center;
    padding: 8px 10px;
    border: 1px solid var(--el-border-color-lighter);
    border-radius: 8px;
    cursor: pointer;
  }
  .pp.on {
    border-color: #5d87ff;
    background: #eff3ff;
  }
  .pp.dis {
    opacity: 0.45;
    cursor: not-allowed;
  }
  .pp__t {
    font-size: 13px;
    font-weight: 600;
  }
  .pp__d {
    font-size: 11px;
    color: #8a919f;
  }
  .pp__p {
    margin-left: auto;
    text-align: right;
    font-weight: 600;
    font-size: 13px;
  }
  .pp__p small {
    display: block;
    font-weight: 400;
    font-size: 11px;
    color: #8a919f;
  }
  .cal {
    display: grid;
    grid-template-columns: repeat(7, 1fr);
    gap: 4px;
  }
  .cal-head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 6px;
  }
  .cal__d {
    min-height: 42px;
    padding: 6px 2px 4px;
    text-align: center;
    font-size: 12px;
    border-radius: 6px;
    border: 1px solid transparent;
    cursor: pointer;
  }
  .cal__d small {
    display: block;
    font-size: 10px;
    color: #8a919f;
    margin-top: 2px;
  }
  .cal__d.past {
    color: #c0c4cc;
    cursor: default;
  }
  .cal__d.full {
    background: #f1f3f6;
    color: #a0a6b1;
    text-decoration: line-through;
    cursor: default;
  }
  .cal__d.sel {
    background: #eff3ff;
    border-color: #aec3ff;
  }
  .order-line {
    display: flex;
    justify-content: space-between;
    font-size: 13px;
    padding: 4px 0;
  }
  .nr-box {
    background: #f4f6fa;
    border: 1px solid #e3e6ed;
    border-radius: 8px;
    padding: 10px 12px;
    font-size: 12px;
    color: #5c6370;
    line-height: 1.6;
    margin: 10px 0;
  }
  .nr-box b {
    color: #1f2329;
  }
  .nr-box ul {
    margin: 6px 0 0;
    padding-left: 18px;
  }
  @media (max-width: 900px) {
    .buy-grid {
      grid-template-columns: 1fr;
    }
  }
</style>
