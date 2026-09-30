<!-- 仓库选项：令牌状态单独一行，三个选择竖排占满宽度，输入框只出现在选中的那一张里。 -->
<template>
  <div class="repo-options">
    <p class="repo-options__token" :class="tokenReady ? 'is-ok' : 'is-bad'">
      <span class="repo-options__dot" />
      {{ tokenMessage }}
    </p>
    <div class="repo-options__list" role="radiogroup">
      <label
        v-for="option in options"
        :key="option.value"
        class="repo-options__card"
        :class="{
          'is-on': modelValue === option.value,
          'is-off': optionNeedsToken(option) && !tokenReady
        }"
      >
        <input
          class="repo-options__input"
          type="radio"
          :value="option.value"
          :checked="modelValue === option.value"
          :disabled="optionNeedsToken(option) && !tokenReady"
          @change="choose(option.value)"
        />
        <span class="repo-options__mark" />
        <span class="repo-options__body">
          <strong>{{ option.title }}</strong>
          <span v-if="option.note && modelValue === option.value" class="repo-options__note">{{
            option.note
          }}</span>
          <span v-if="showField(option)" class="repo-options__field" @click.stop>
            <span class="repo-options__label">{{ option.field }}</span>
            <ElInput
              :model-value="repo"
              :placeholder="option.placeholder || '所有者/仓库'"
              @update:model-value="onRepo"
            />
            <span v-if="hint" class="repo-options__note">{{ hint }}</span>
          </span>
        </span>
      </label>
    </div>
  </div>
</template>

<script setup lang="ts">
  defineOptions({ name: 'RepoOptionCards' })

  export interface RepoOption {
    value: string
    title: string
    /** 有字段时，选中后把输入框放进这张卡片。 */
    field?: string
    placeholder?: string
    note?: string
    needsToken?: boolean
  }

  const props = defineProps<{
    modelValue: string
    repo: string
    tokenReady: boolean
    tokenMessage: string
    hint?: string
    options: RepoOption[]
  }>()

  const emit = defineEmits<{
    'update:modelValue': [string]
    'update:repo': [string]
    touched: []
  }>()

  function optionNeedsToken(option: RepoOption) {
    return option.needsToken !== false && option.value !== 'skip'
  }

  function choose(value: string) {
    emit('update:modelValue', value)
  }

  function showField(option: RepoOption) {
    return !!option.field && props.modelValue === option.value && props.tokenReady
  }

  function onRepo(value: string) {
    emit('update:repo', value.trim())
    emit('touched')
  }
</script>

<style scoped>
  .repo-options__token {
    display: flex;
    gap: 8px;
    align-items: flex-start;
    margin: 0 0 16px;
    padding: 10px 12px;
    font-size: 13px;
    line-height: 1.5;
    border-radius: 8px;
  }

  .repo-options__token.is-ok {
    color: #1f7a4d;
    background: #eef8f2;
  }

  .repo-options__token.is-bad {
    color: #b44848;
    background: #fef0f0;
  }

  .repo-options__dot {
    flex: none;
    width: 8px;
    height: 8px;
    margin-top: 6px;
    border-radius: 50%;
    background: currentcolor;
  }

  .repo-options__list {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .repo-options__card {
    display: flex;
    gap: 12px;
    align-items: flex-start;
    padding: 14px 16px;
    cursor: pointer;
    background: #fff;
    border: 1px solid #e6eaf0;
    border-radius: 10px;
  }

  .repo-options__card.is-on {
    border-color: #5d87ff;
    box-shadow: inset 0 0 0 1px #5d87ff;
  }

  .repo-options__card.is-off {
    cursor: not-allowed;
    opacity: 0.55;
  }

  .repo-options__input {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip: rect(0 0 0 0);
  }

  .repo-options__mark {
    flex: none;
    width: 16px;
    height: 16px;
    margin-top: 2px;
    border: 1px solid #c5ced9;
    border-radius: 50%;
  }

  .repo-options__card.is-on .repo-options__mark {
    border: 5px solid #5d87ff;
  }

  .repo-options__body {
    display: flex;
    flex: 1;
    flex-direction: column;
    gap: 8px;
    min-width: 0;
  }

  .repo-options__body strong {
    color: #172033;
    font-size: 14px;
    font-weight: 600;
    line-height: 22px;
  }

  .repo-options__field {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .repo-options__label,
  .repo-options__note {
    color: #6b7686;
    font-size: 12px;
    line-height: 1.5;
  }
</style>
