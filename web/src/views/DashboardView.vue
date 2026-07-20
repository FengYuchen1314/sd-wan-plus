<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { api, connectWS } from '../api'

const data = ref<Record<string, any>>({})
const error = ref('')
let ws: WebSocket | null = null

async function load() {
  try {
    data.value = await api.dashboard()
  } catch (e: any) {
    error.value = e.message
  }
}

onMounted(() => {
  load()
  ws = connectWS(() => load())
})
onUnmounted(() => ws?.close())
</script>

<template>
  <div>
    <h1 class="page-title">总览</h1>
    <p class="page-sub">控制树健康、数据图链路与配置一致性一览</p>
    <p v-if="error" class="error">{{ error }}</p>
    <div class="grid-stats">
      <div class="card"><div class="stat-value">{{ data.node_total ?? '—' }}</div><div class="stat-label">节点总数</div></div>
      <div class="card"><div class="stat-value">{{ data.agent_online ?? '—' }}</div><div class="stat-label">Agent 在线</div></div>
      <div class="card"><div class="stat-value">{{ data.overlay_reachable ?? '—' }}</div><div class="stat-label">Overlay 可达</div></div>
      <div class="card"><div class="stat-value">{{ data.link_total ?? '—' }}</div><div class="stat-label">WireGuard 链路</div></div>
      <div class="card"><div class="stat-value">{{ data.link_failed ?? '—' }}</div><div class="stat-label">失败链路</div></div>
      <div class="card"><div class="stat-value mono" style="font-size:1.1rem">{{ data.version ?? '—' }}</div><div class="stat-label">软件版本</div></div>
    </div>
  </div>
</template>
