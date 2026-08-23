<template>
  <RemoteHomeTemplate v-if="activeDocument" :key="activeTemplateKey" :document="activeDocument" />
  <DefaultHomeTemplate v-else />
</template>

<script setup lang="ts">
  import { onErrorCaptured, onMounted, ref, shallowRef } from 'vue'
  import axios from 'axios'
  import DefaultHomeTemplate from './DefaultHomeTemplate.vue'
  import RemoteHomeTemplate from './RemoteHomeTemplate.vue'
  import {
    type ActiveHomeTemplateResponse,
    type HomeTemplateDocument,
    isHomeTemplateDocument
  } from './home-template'

  defineOptions({ name: 'UserLogin' })

  const activeDocument = shallowRef<HomeTemplateDocument | null>(null)
  const activeTemplateKey = ref('default')

  function useDefaultTemplate() {
    activeDocument.value = null
    activeTemplateKey.value = 'default'
  }

  async function loadActiveTemplate() {
    try {
      const { data } = await axios.get('/api/home-template/active', { timeout: 5000 })
      const result = data?.data as ActiveHomeTemplateResponse | undefined
      if (data?.code !== 200 || !result || result.isDefault) {
        useDefaultTemplate()
        return
      }
      if (!isHomeTemplateDocument(result.document)) {
        useDefaultTemplate()
        return
      }
      activeTemplateKey.value = `${result.id}:${result.version}`
      activeDocument.value = result.document
    } catch {
      useDefaultTemplate()
    }
  }

  onErrorCaptured((error) => {
    if (!activeDocument.value) return
    console.error('[HomeTemplate] 远程首页模板渲染失败，已回退默认模板', error)
    useDefaultTemplate()
    return false
  })

  onMounted(loadActiveTemplate)
</script>
