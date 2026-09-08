<template>
  <RemoteHomeTemplate
    v-if="activeDocument"
    :key="activeTemplateKey"
    :document="activeDocument"
    :static-entry-url="staticEntryUrl"
  />
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
    isHomeTemplateDocument,
    isStaticHomeTemplateEntryURL,
    resolveHomeTemplateAssets
  } from './home-template'

  defineOptions({ name: 'UserLogin' })

  const activeDocument = shallowRef<HomeTemplateDocument | null>(null)
  const activeTemplateKey = ref('default')
  const staticEntryUrl = ref('')

  function useDefaultTemplate() {
    activeDocument.value = null
    staticEntryUrl.value = ''
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
      if (result.format === 'static') {
        if (!isStaticHomeTemplateEntryURL(result.entryUrl)) {
          useDefaultTemplate()
          return
        }
        staticEntryUrl.value = result.entryUrl
        activeTemplateKey.value = `${result.id}:${result.version}:${result.entryUrl}`
        activeDocument.value = { schemaVersion: 1, hero: { title: result.name || '自定义首页' } }
        return
      }
      if (!isHomeTemplateDocument(result.document)) {
        useDefaultTemplate()
        return
      }
      activeTemplateKey.value = `${result.id}:${result.version}`
      staticEntryUrl.value = ''
      activeDocument.value = resolveHomeTemplateAssets(result.document, result.assetBaseUrl)
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
