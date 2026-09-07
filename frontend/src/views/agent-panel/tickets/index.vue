<template>
  <PanelTicketCenter
    api-prefix="/api/agent-panel"
    token-key="agent_panel_token"
    self-type="agent"
  />
</template>

<script setup lang="ts">
  import PanelTicketCenter from '@/components/core/panels/PanelTicketCenter.vue'
</script>
