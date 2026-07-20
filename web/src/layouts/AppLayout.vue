<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { RouterLink, RouterView, useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import { connectWS } from '../api'

const auth = useAuthStore()
const router = useRouter()
const live = ref(false)
let ws: WebSocket | null = null

const nav = [
  { to: '/', label: '总览' },
  { to: '/topology', label: '拓扑' },
  { to: '/nodes', label: '节点' },
  { to: '/enrollment', label: '接入' },
  { to: '/links', label: '链路' },
  { to: '/policies', label: '路径策略' },
  { to: '/publish', label: '配置发布' },
  { to: '/updates', label: '更新中心' },
  { to: '/settings', label: '设置' },
]

async function logout() {
  await auth.logout()
  router.push({ name: 'login' })
}

onMounted(() => {
  ws = connectWS(() => { live.value = true })
  ws.onopen = () => { live.value = true }
  ws.onclose = () => { live.value = false }
})
onUnmounted(() => ws?.close())
</script>

<template>
  <div class="shell">
    <aside class="side">
      <div class="brand">
        <div class="mark" />
        <div>
          <div class="brand-name">PathWeaver</div>
          <div class="brand-sub">SD-WAN Control</div>
        </div>
      </div>
      <nav>
        <RouterLink v-for="item in nav" :key="item.to" :to="item.to" class="nav-item" exact-active-class="active">
          {{ item.label }}
        </RouterLink>
      </nav>
      <div class="side-foot">
        <span class="live" :class="{ on: live }" /> {{ live ? '实时连接' : '离线' }}
        <button class="secondary" style="width:100%;margin-top:0.75rem" @click="logout">退出</button>
      </div>
    </aside>
    <main class="main">
      <RouterView />
    </main>
  </div>
</template>

<style scoped>
.shell { display: grid; grid-template-columns: 220px 1fr; min-height: 100%; }
.side {
  background: linear-gradient(180deg, #0f1824 0%, #0c1219 100%);
  border-right: 1px solid var(--border);
  padding: 1.25rem 0.9rem;
  display: flex; flex-direction: column; gap: 1.25rem;
}
.brand { display: flex; gap: 0.75rem; align-items: center; padding: 0 0.35rem; }
.mark {
  width: 34px; height: 34px; border-radius: 9px;
  background: linear-gradient(135deg, #2dd4a8, #4da3ff);
  box-shadow: 0 0 24px color-mix(in srgb, var(--accent) 35%, transparent);
}
.brand-name { font-weight: 700; letter-spacing: -0.03em; }
.brand-sub { color: var(--muted); font-size: 0.72rem; }
nav { display: flex; flex-direction: column; gap: 0.2rem; flex: 1; }
.nav-item {
  color: var(--muted); padding: 0.55rem 0.7rem; border-radius: 8px; text-decoration: none;
}
.nav-item:hover { background: #152033; color: var(--text); text-decoration: none; }
.nav-item.active { background: #163328; color: var(--accent); font-weight: 600; }
.side-foot { color: var(--muted); font-size: 0.8rem; padding: 0 0.35rem; }
.live {
  display: inline-block; width: 8px; height: 8px; border-radius: 50%;
  background: #556; margin-right: 0.35rem;
}
.live.on { background: var(--accent); box-shadow: 0 0 8px var(--accent); }
.main { padding: 1.5rem 1.75rem; overflow: auto; }
@media (max-width: 860px) {
  .shell { grid-template-columns: 1fr; }
  .side { border-right: none; border-bottom: 1px solid var(--border); }
  nav { flex-direction: row; flex-wrap: wrap; }
}
</style>
