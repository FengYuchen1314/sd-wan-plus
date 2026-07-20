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
const nodeMeta = ref<Record<string, { wgStart: number; wgEnd: number; public: boolean; suggestAddr: string }>>({})

const linkDialogOpen = ref(false)
const linkBusy = ref(false)
const linkAddr = ref('')
const linkPort = ref(14303)
const linkShowPort = ref(false)

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

function isPrivateIPv4(host: string) {
  const m = host.trim().match(/^(\d+)\.(\d+)\.(\d+)\.(\d+)$/)
  if (!m) return false
  const a = +m[1], b = +m[2]
  if (a === 10 || a === 127) return true
  if (a === 192 && b === 168) return true
  if (a === 172 && b >= 16 && b <= 31) return true
  if (a === 100 && b >= 64 && b <= 127) return true
  return false
}

function nodeIsPublic(addrs: any[] | undefined) {
  if (!addrs?.length) return false
  for (const a of addrs) {
    const t = (a.address_type || '').toLowerCase()
    if (t === 'lan') continue
    const host = String(a.address || '').trim()
    if (!host) continue
    if (t === 'public' || !isPrivateIPv4(host)) return true
  }
  return false
}

function suggestAddress(addrs: any[] | undefined) {
  if (!addrs?.length) return ''
  const pub = addrs.find((a) => (a.address_type || '').toLowerCase() === 'public')
  if (pub?.address) return pub.address
  const primary = addrs.find((a) => a.is_primary)
  if (primary?.address) return primary.address
  return addrs[0]?.address || ''
}

async function load() {
  const g = await api.topology()
  if (!el.value) return
  const names: Record<string, string> = {}
  const meta: Record<string, { wgStart: number; wgEnd: number; public: boolean; suggestAddr: string }> = {}
  for (const n of g.nodes || []) {
    names[n.id] = n.display_name
    const addrs = (g.addresses && g.addresses[n.id]) || []
    meta[n.id] = {
      wgStart: n.wg_port_range_start || 14303,
      wgEnd: n.wg_port_range_end || n.wg_port_range_start || 14303,
      public: nodeIsPublic(addrs),
      suggestAddr: suggestAddress(addrs),
    }
  }
  nodeNames.value = names
  nodeMeta.value = meta

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

function openLinkDialog() {
  if (selected.value.length !== 2) return
  const [, b] = selected.value // first=initiator, second=listener (passive)
  const meta = nodeMeta.value[b]
  linkShowPort.value = !(meta?.public)
  linkAddr.value = meta?.suggestAddr || ''
  linkPort.value = meta?.wgStart || 14303
  linkDialogOpen.value = true
  error.value = ''
}

function closeLinkDialog() {
  linkDialogOpen.value = false
}

async function submitLink() {
  if (selected.value.length !== 2) return
  const addr = linkAddr.value.trim()
  if (!addr) {
    error.value = '请填写被动端可达地址'
    return
  }
  if (linkShowPort.value) {
    const p = Number(linkPort.value)
    if (!Number.isInteger(p) || p < 1 || p > 65535) {
      error.value = '监听端口无效'
      return
    }
  }
  const [a, b] = selected.value
  linkBusy.value = true
  error.value = ''
  try {
    const body: any = {
      node_a: a, node_b: b, initiator_node_id: a,
      listener_address: addr, enabled: true, bidirectional: false,
    }
    // LAN listener: always send port (default from DB). Public: omit → server AllocateWGPort.
    if (linkShowPort.value) {
      body.listener_port = Number(linkPort.value)
    }
    await api.createLink(body)
    msg.value = '链路已创建（单向）'
    linkDialogOpen.value = false
    await load()
  } catch (e: any) {
    error.value = e.message
  } finally {
    linkBusy.value = false
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
      <button :disabled="selected.length !== 2" @click="openLinkDialog">建立 WireGuard 链路</button>
      <button :disabled="selected.length !== 2" class="secondary" @click="openPathPanel">查看路径</button>
      <button class="secondary" @click="load">刷新</button>
      <span class="mono" style="color:var(--muted)">已选 {{ selected.length }}/2</span>
      <span v-if="selected.length" class="mono" style="color:var(--muted)">{{ selectedLabels.join(' · ') }}</span>
    </div>
    <p v-if="msg" class="success">{{ msg }}</p>
    <p v-if="error" class="error">{{ error }}</p>
    <div ref="el" class="topo-wrap" />

    <div v-if="linkDialogOpen" class="modal-backdrop" @click.self="closeLinkDialog">
      <div class="modal card" role="dialog" aria-modal="true">
        <h2 style="margin-bottom:0.5rem">建立 WireGuard 链路</h2>
        <p class="page-sub" style="margin-bottom:0.75rem">
          单向：{{ selectedLabels[0] }} → {{ selectedLabels[1] }}（先选主动，后选被动）
        </p>
        <div class="form-row">
          <label>被动端可达地址</label>
          <input v-model="linkAddr" placeholder="IP / 域名（端口映射时填对外地址）" />
        </div>
        <div v-if="linkShowPort" class="form-row">
          <label>被动端监听端口</label>
          <input v-model.number="linkPort" type="number" min="1" max="65535" />
          <p class="page-sub" style="margin:0.35rem 0 0">默认取节点库中端口；有端口映射时可改成对外端口</p>
        </div>
        <div class="modal-actions" style="margin-top:1rem">
          <button :disabled="linkBusy" @click="submitLink">{{ linkBusy ? '创建中…' : '创建' }}</button>
          <span style="flex:1"></span>
          <button class="secondary" :disabled="linkBusy" @click="closeLinkDialog">取消</button>
        </div>
      </div>
    </div>

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
.modal-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.55);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 50;
  padding: 1rem;
}
.modal {
  width: min(440px, 100%);
  max-height: 90vh;
  overflow: auto;
  padding: 1rem;
}
.modal-actions {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}
</style>
