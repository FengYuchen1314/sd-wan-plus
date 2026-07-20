<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, shallowRef } from 'vue'
import cytoscape from 'cytoscape'
import { api, connectWS } from '../api'

const overview = ref<any>(null)
const pulling = ref(false)
const error = ref('')
const msg = ref('')
const el = ref<HTMLElement | null>(null)
const cy = shallowRef<cytoscape.Core | null>(null)
let ws: WebSocket | null = null
let pollTimer: number | null = null

const statusColor: Record<string, string> = {
  Idle: '#5a6a7a',
  Waiting: '#5b8def',
  Prefetching: '#4da3ff',
  Verifying: '#4da3ff',
  Staged: '#e8a838',
  Installing: '#e8a838',
  Restarting: '#e8a838',
  HealthChecking: '#e8a838',
  Completed: '#2dd4a8',
  DownloadFailed: '#e85d5d',
  SignatureInvalid: '#e85d5d',
  InstallFailed: '#e85d5d',
  HealthCheckFailed: '#e85d5d',
  RollingBack: '#e8a838',
  RolledBack: '#8fa3b8',
}

const statusLabel: Record<string, string> = {
  Idle: '空闲',
  Waiting: '等待',
  Prefetching: '预取中',
  Verifying: '校验中',
  Staged: '已就绪',
  Installing: '安装中',
  Restarting: '重启中',
  HealthChecking: '健康检查',
  Completed: '已完成',
  DownloadFailed: '下载失败',
  SignatureInvalid: '签名无效',
  InstallFailed: '安装失败',
  HealthCheckFailed: '健康检查失败',
  RollingBack: '回滚中',
  RolledBack: '已回滚',
}

const latest = computed(() => overview.value?.latest || null)
const activeJob = computed(() => overview.value?.active_job || null)
const phase = computed(() => overview.value?.phase || '')
const nodes = computed(() => overview.value?.nodes || [])
const jobs = computed(() => overview.value?.jobs || [])

const summary = computed(() => {
  const list = nodes.value as any[]
  const counts: Record<string, number> = {}
  for (const n of list) {
    const s = n.update_status || 'Idle'
    counts[s] = (counts[s] || 0) + 1
  }
  return counts
})

function labelOf(status: string) {
  return statusLabel[status] || status
}

function colorOf(status: string) {
  return statusColor[status] || '#5a6a7a'
}

async function load(opts: { refreshGithub?: boolean; skipGithub?: boolean } = {}) {
  try {
    const q = opts.refreshGithub ? '?refresh_github=1' : opts.skipGithub ? '?skip_github=1' : ''
    overview.value = await api.updatesOverview(q)
    await nextTick()
    renderTopo()
  } catch (e: any) {
    error.value = e.message
  }
}

function renderTopo() {
  const g = overview.value?.topology
  if (!el.value || !g) return

  const statusMap = new Map<string, any>()
  for (const n of nodes.value) statusMap.set(n.id, n)

  const elements: cytoscape.ElementDefinition[] = []
  for (const n of g.nodes || []) {
    const st = statusMap.get(n.id)
    const status = st?.update_status || 'Idle'
    const online = st?.online
    const ver = st?.agent_version || '—'
    elements.push({
      data: {
        id: n.id,
        label: `${n.display_name}\n${labelOf(status)}\n${ver}`,
        controller: n.is_controller,
        status,
        color: colorOf(status),
        online: online ? 1 : 0,
      },
    })
  }
  for (const r of g.control_relations || []) {
    elements.push({
      data: { id: `c-${r.id}`, source: r.parent_id, target: r.child_id, kind: 'control' },
    })
  }

  const existing = cy.value
  if (existing) {
    // in-place style/label update when possible
    for (const n of elements.filter((e) => !e.data.source)) {
      const node = existing.getElementById(n.data.id as string)
      if (node.nonempty()) {
        node.data(n.data)
      }
    }
    existing.style().selector('node').style({
      'background-color': 'data(color)',
      'border-color': '#2a3a4a',
      'border-width': 2,
      label: 'data(label)',
      color: '#e8eef6',
      'font-size': 10,
      'text-wrap': 'wrap',
      'text-valign': 'bottom',
      'text-margin-y': 8,
      width: 32,
      height: 32,
      'text-max-width': '90px',
    } as any).update()
    return
  }

  cy.value = cytoscape({
    container: el.value,
    elements,
    style: [
      {
        selector: 'node',
        style: {
          'background-color': 'data(color)',
          'border-color': '#243044',
          'border-width': 2,
          label: 'data(label)',
          color: '#e8eef6',
          'font-size': 10,
          'text-wrap': 'wrap',
          'text-valign': 'bottom',
          'text-margin-y': 8,
          width: 32,
          height: 32,
          'text-max-width': '90px',
        } as any,
      },
      {
        selector: 'node[controller]',
        style: { 'border-color': '#4da3ff', 'border-width': 3 },
      },
      {
        selector: 'node[online = 0]',
        style: { opacity: 0.45 },
      },
      {
        selector: 'edge[kind = "control"]',
        style: {
          width: 2,
          'line-color': '#5b8def',
          'line-style': 'dashed',
          'target-arrow-color': '#5b8def',
          'target-arrow-shape': 'triangle',
          'curve-style': 'bezier',
        },
      },
    ],
    layout: { name: 'breadthfirst', directed: true, padding: 36, spacingFactor: 1.35 },
  })
}

async function pullAndStart() {
  pulling.value = true
  error.value = ''
  msg.value = ''
  try {
    const res: any = await api.pullLatest(true)
    msg.value = `已拉取 ${res.latest?.version || res.job?.target_version}，更新任务已启动`
    await load()
  } catch (e: any) {
    error.value = e.message
  } finally {
    pulling.value = false
  }
}

async function start(id: string) {
  await api.startUpdate(id)
  msg.value = '更新任务已启动'
  await load()
}

async function rollback(id: string) {
  await api.rollbackUpdate(id)
  msg.value = '已标记回滚'
  await load()
}

onMounted(() => {
  load()
  ws = connectWS((m) => {
    if (m?.event === 'updates') load({ skipGithub: true })
  })
  pollTimer = window.setInterval(() => {
    if (activeJob.value?.status === 'Running' || phase.value) load({ skipGithub: true })
  }, 4000)
})

onUnmounted(() => {
  ws?.close()
  if (pollTimer) clearInterval(pollTimer)
  cy.value?.destroy()
})
</script>

<template>
  <div>
    <h1 class="page-title">更新中心</h1>
    <p class="page-sub">
      自动从 GitHub Latest 拉取制品；沿控制树预分发，叶子优先安装。拓扑图实时显示各节点更新状态。
    </p>

    <div class="grid-stats" style="margin-bottom:1rem">
      <div class="card">
        <div class="stat-label">当前控制机版本</div>
        <div class="stat-value" style="font-size:1.15rem">{{ overview?.current_version || '—' }}</div>
      </div>
      <div class="card">
        <div class="stat-label">GitHub Latest</div>
        <div class="stat-value" style="font-size:1.15rem">
          {{ latest?.version || latest?.error || '检测中…' }}
        </div>
        <div class="stat-label" v-if="latest?.outdated" style="color:var(--warn)">有新版本</div>
        <div class="stat-label" v-else-if="latest?.version">已是最新</div>
      </div>
      <div class="card">
        <div class="stat-label">进行中任务</div>
        <div class="stat-value" style="font-size:1.15rem">
          {{ activeJob ? activeJob.target_version : '无' }}
        </div>
        <div class="stat-label" v-if="phase">阶段: {{ phase }}</div>
      </div>
      <div class="card">
        <div class="stat-label">节点进度</div>
        <div style="display:flex;flex-wrap:wrap;gap:0.35rem;margin-top:0.4rem">
          <span v-for="(n, s) in summary" :key="s" class="badge" :style="{ color: colorOf(String(s)) }">
            {{ labelOf(String(s)) }} {{ n }}
          </span>
        </div>
      </div>
    </div>

    <div class="row-actions" style="margin-bottom:1rem">
      <button :disabled="pulling" @click="pullAndStart">
        {{ pulling ? '拉取中…' : '拉取最新并开始更新' }}
      </button>
      <button class="secondary" @click="load({ refreshGithub: true })">刷新</button>
    </div>
    <p v-if="msg" class="success">{{ msg }}</p>
    <p v-if="error" class="error">{{ error }}</p>

    <div class="card" style="margin-bottom:1rem;padding:0.75rem">
      <div class="legend">
        <span v-for="(c, s) in statusColor" :key="s" class="legend-item">
          <i :style="{ background: c }" />{{ labelOf(String(s)) }}
        </span>
      </div>
      <div ref="el" class="topo-wrap" style="height:min(62vh,560px);border:none;margin-top:0.5rem" />
    </div>

    <div class="card" style="overflow:auto;margin-bottom:1rem">
      <table class="data">
        <thead>
          <tr>
            <th>节点</th>
            <th>版本</th>
            <th>在线</th>
            <th>深度</th>
            <th>更新状态</th>
            <th>错误</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="n in nodes" :key="n.id">
            <td>
              {{ n.display_name }}
              <span v-if="n.is_controller" class="badge ok" style="margin-left:0.35rem">控制机</span>
            </td>
            <td class="mono">{{ n.agent_version || '—' }}</td>
            <td>
              <span class="badge" :class="n.online ? 'ok' : 'err'">{{ n.online ? '在线' : '离线' }}</span>
            </td>
            <td>{{ n.depth }}</td>
            <td>
              <span class="badge" :style="{ color: colorOf(n.update_status) }">
                {{ labelOf(n.update_status) }}
              </span>
            </td>
            <td>{{ n.error_message || '—' }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="card" style="overflow:auto">
      <table class="data">
        <thead>
          <tr><th>版本</th><th>状态</th><th>创建时间</th><th></th></tr>
        </thead>
        <tbody>
          <tr v-for="j in jobs" :key="j.id">
            <td class="mono">{{ j.target_version }}</td>
            <td><span class="badge" :class="j.status === 'Completed' ? 'ok' : j.status === 'Running' ? 'warn' : ''">{{ j.status }}</span></td>
            <td class="mono">{{ j.created_at }}</td>
            <td class="row-actions">
              <button v-if="j.status !== 'Running' && j.status !== 'Completed'" @click="start(j.id)">启动</button>
              <button class="danger" @click="rollback(j.id)">回滚</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.legend {
  display: flex;
  flex-wrap: wrap;
  gap: 0.55rem 0.9rem;
  color: var(--muted);
  font-size: 0.75rem;
}
.legend-item { display: inline-flex; align-items: center; gap: 0.35rem; }
.legend-item i {
  width: 10px; height: 10px; border-radius: 50%; display: inline-block;
}
</style>
