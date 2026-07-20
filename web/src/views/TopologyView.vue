<script setup lang="ts">
import { computed, onMounted, ref, shallowRef, watch } from 'vue'
import cytoscape from 'cytoscape'
import { api } from '../api'

const el = ref<HTMLElement | null>(null)
const cy = shallowRef<cytoscape.Core | null>(null)
const selected = ref<string[]>([])
const msg = ref('')
const error = ref('')
const nodeNames = ref<Record<string, string>>({})

const pathPanelOpen = ref(false)
const pathList = ref<string[][]>([])
const pickedIdx = ref<number | null>(null)
const preferred = ref<{ id: string; hops: string[] } | null>(null)
const pathBusy = ref(false)

const selectedLabels = computed(() =>
  selected.value.map((id) => nodeNames.value[id] || id.slice(0, 8)),
)

function pathLabel(hops: string[]) {
  return hops.map((id) => nodeNames.value[id] || id.slice(0, 8)).join(' → ')
}

function hopsEqual(a: string[], b: string[]) {
  if (a.length !== b.length) return false
  return a.every((x, i) => x === b[i])
}

function clearPathHighlight() {
  cy.value?.edges().removeClass('path-hl')
}

function highlightPath(hops: string[]) {
  clearPathHighlight()
  if (!cy.value || hops.length < 2) return
  for (let i = 0; i < hops.length - 1; i++) {
    const a = hops[i]
    const b = hops[i + 1]
    cy.value.edges().forEach((edge) => {
      if (edge.data('kind') !== 'data') return
      const s = edge.data('source')
      const t = edge.data('target')
      if ((s === a && t === b) || (s === b && t === a)) {
        edge.addClass('path-hl')
      }
    })
  }
}

async function load() {
  const g = await api.topology()
  if (!el.value) return
  const names: Record<string, string> = {}
  for (const n of g.nodes || []) {
    names[n.id] = n.display_name
  }
  nodeNames.value = names

  cy.value?.destroy()
  const elements: cytoscape.ElementDefinition[] = []
  for (const n of g.nodes || []) {
    elements.push({
      data: { id: n.id, label: n.display_name, controller: n.is_controller },
    })
  }
  for (const r of g.control_relations || []) {
    elements.push({
      data: { id: `c-${r.id}`, source: r.parent_id, target: r.child_id, kind: 'control' },
    })
  }
  for (const l of g.links || []) {
    elements.push({
      data: {
        id: `l-${l.id}`, source: l.node_a, target: l.node_b, kind: 'data',
        label: `${l.listener_port}`,
      },
    })
  }
  cy.value = cytoscape({
    container: el.value,
    elements,
    style: [
      { selector: 'node', style: {
        'background-color': '#1e3a4f', 'border-color': '#2dd4a8', 'border-width': 2,
        label: 'data(label)', color: '#e8eef6', 'font-size': 11, 'text-valign': 'bottom', 'text-margin-y': 6,
        width: 28, height: 28,
      }},
      { selector: 'node[controller]', style: { 'background-color': '#2a4a7a', 'border-color': '#4da3ff' } },
      { selector: 'node:selected', style: { 'border-color': '#e8a838', 'border-width': 3 } },
      { selector: 'edge[kind = "control"]', style: {
        width: 2, 'line-color': '#5b8def', 'line-style': 'dashed',
        'target-arrow-color': '#5b8def', 'target-arrow-shape': 'triangle', 'curve-style': 'bezier',
      }},
      { selector: 'edge[kind = "data"]', style: {
        width: 3, 'line-color': '#2dd4a8', 'target-arrow-color': '#2dd4a8',
        'target-arrow-shape': 'none', 'curve-style': 'bezier', label: 'data(label)', 'font-size': 9, color: '#8fa3b8',
      }},
      { selector: 'edge.path-hl', style: {
        width: 5, 'line-color': '#e8a838', 'target-arrow-color': '#e8a838',
      }},
    ],
    layout: { name: 'cose', animate: false, padding: 40 },
  })
  cy.value.on('tap', 'node', (evt) => {
    const id = evt.target.id()
    if (selected.value.includes(id)) {
      selected.value = selected.value.filter((x) => x !== id)
    } else if (selected.value.length >= 2) {
      selected.value = [selected.value[1], id]
    } else {
      selected.value = [...selected.value, id]
    }
  })
  if (pathPanelOpen.value && selected.value.length === 2) {
    await refreshPathPanel()
  }
}

async function createLink() {
  if (selected.value.length !== 2) return
  const [a, b] = selected.value
  const addr = prompt('被动端可达 IP/域名（默认单向：先选节点主动拨后选节点）')
  if (!addr) return
  try {
    await api.createLink({
      node_a: a, node_b: b, initiator_node_id: a,
      listener_address: addr, enabled: true, bidirectional: false,
    })
    msg.value = '链路已创建（单向）'
    error.value = ''
    await load()
  } catch (e: any) {
    error.value = e.message
  }
}

async function refreshPathPanel() {
  if (selected.value.length !== 2) return
  const [from, to] = selected.value
  pathBusy.value = true
  error.value = ''
  try {
    const [listed, pref] = await Promise.all([
      api.topologyPaths(from, to),
      api.listOverlayPaths(from, to),
    ])
    pathList.value = listed.paths || []
    const cur = (pref.paths || [])[0]
    if (cur?.id && cur.hops) {
      preferred.value = { id: cur.id, hops: cur.hops }
      const idx = pathList.value.findIndex((p) => hopsEqual(p, cur.hops) || hopsEqual([...p].reverse(), cur.hops))
      pickedIdx.value = idx >= 0 ? idx : null
      highlightPath(cur.hops[0] === from ? cur.hops : [...cur.hops].reverse())
    } else {
      preferred.value = null
      pickedIdx.value = pathList.value.length ? 0 : null
      if (pathList.value[0]) highlightPath(pathList.value[0])
      else clearPathHighlight()
    }
  } catch (e: any) {
    error.value = e.message
    pathList.value = []
    preferred.value = null
  } finally {
    pathBusy.value = false
  }
}

async function openPathPanel() {
  if (selected.value.length !== 2) return
  pathPanelOpen.value = true
  await refreshPathPanel()
}

function closePathPanel() {
  pathPanelOpen.value = false
  pickedIdx.value = null
  pathList.value = []
  preferred.value = null
  clearPathHighlight()
}

function selectPath(i: number) {
  pickedIdx.value = i
  if (pathList.value[i]) highlightPath(pathList.value[i])
}

async function setPreferredPath() {
  if (pickedIdx.value === null || !pathList.value[pickedIdx.value]) return
  const hops = pathList.value[pickedIdx.value]
  pathBusy.value = true
  error.value = ''
  try {
    await api.createOverlayPath({
      src_node_id: hops[0],
      dst_node_id: hops[hops.length - 1],
      hops,
    })
    msg.value = '已设为通信路径并发布'
    await refreshPathPanel()
  } catch (e: any) {
    error.value = e.message
  } finally {
    pathBusy.value = false
  }
}

async function clearPreferredPath() {
  if (!preferred.value?.id) return
  pathBusy.value = true
  error.value = ''
  try {
    await api.deleteOverlayPath(preferred.value.id)
    msg.value = '已恢复默认最短路径并发布'
    await refreshPathPanel()
  } catch (e: any) {
    error.value = e.message
  } finally {
    pathBusy.value = false
  }
}

watch(selected, (v) => {
  if (v.length !== 2 && pathPanelOpen.value) {
    closePathPanel()
  }
})

onMounted(load)
</script>

<template>
  <div>
    <h1 class="page-title">拓扑</h1>
    <p class="page-sub">虚线=控制树 · 实线=WireGuard 数据图 · 选两个节点可建链路或查看路径</p>
    <div class="row-actions" style="margin-bottom:0.75rem">
      <button :disabled="selected.length !== 2" @click="createLink">建立 WireGuard 链路</button>
      <button :disabled="selected.length !== 2" class="secondary" @click="openPathPanel">查看路径</button>
      <button class="secondary" @click="load">刷新</button>
      <span class="mono" style="color:var(--muted)">已选 {{ selected.length }}/2</span>
      <span v-if="selected.length" class="mono" style="color:var(--muted)">{{ selectedLabels.join(' · ') }}</span>
    </div>
    <p v-if="msg" class="success">{{ msg }}</p>
    <p v-if="error" class="error">{{ error }}</p>
    <div ref="el" class="topo-wrap" />

    <div v-if="pathPanelOpen" class="path-panel card" style="margin-top:1rem">
      <div class="row-actions" style="margin-bottom:0.5rem; justify-content:space-between">
        <strong>路径详情：{{ selectedLabels.join(' ↔ ') }}</strong>
        <button class="secondary" type="button" @click="closePathPanel">关闭</button>
      </div>
      <p v-if="pathBusy" class="page-sub">加载中…</p>
      <p v-else-if="!pathList.length" class="page-sub">无可用无环路径（需已启用 WireGuard 边连通）</p>
      <ul v-else class="path-list">
        <li
          v-for="(hops, i) in pathList"
          :key="i"
          :class="{ active: pickedIdx === i, preferred: preferred && hopsEqual(hops, preferred.hops) }"
          @click="selectPath(i)"
        >
          <span class="mono">{{ hops.length - 1 }} 跳</span>
          <span>{{ pathLabel(hops) }}</span>
          <span v-if="preferred && (hopsEqual(hops, preferred.hops) || hopsEqual([...hops].reverse(), preferred.hops))" class="badge">当前</span>
        </li>
      </ul>
      <div class="row-actions" style="margin-top:0.75rem">
        <button :disabled="pathBusy || pickedIdx === null" @click="setPreferredPath">设为通信路径</button>
        <button class="secondary" :disabled="pathBusy || !preferred" @click="clearPreferredPath">恢复默认</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.path-panel {
  padding: 1rem;
}
.path-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}
.path-list li {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.55rem 0.75rem;
  border: 1px solid var(--border, #2a3a4a);
  border-radius: 6px;
  cursor: pointer;
}
.path-list li.active {
  border-color: #e8a838;
  background: rgba(232, 168, 56, 0.08);
}
.path-list li .badge {
  margin-left: auto;
  font-size: 0.75rem;
  color: #e8a838;
}
</style>
