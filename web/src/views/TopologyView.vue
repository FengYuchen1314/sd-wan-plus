<script setup lang="ts">
import { onMounted, ref, shallowRef } from 'vue'
import cytoscape from 'cytoscape'
import { api } from '../api'

const el = ref<HTMLElement | null>(null)
const cy = shallowRef<cytoscape.Core | null>(null)
const selected = ref<string[]>([])
const msg = ref('')
const error = ref('')

async function load() {
  const g = await api.topology()
  if (!el.value) return
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
    await load()
  } catch (e: any) {
    error.value = e.message
  }
}

onMounted(load)
</script>

<template>
  <div>
    <h1 class="page-title">拓扑</h1>
    <p class="page-sub">虚线=控制树 · 实线=WireGuard 数据图 · 选择两个节点可建链路</p>
    <div class="row-actions" style="margin-bottom:0.75rem">
      <button :disabled="selected.length !== 2" @click="createLink">建立 WireGuard 链路</button>
      <button class="secondary" @click="load">刷新</button>
      <span class="mono" style="color:var(--muted)">已选 {{ selected.length }}/2</span>
    </div>
    <p v-if="msg" class="success">{{ msg }}</p>
    <p v-if="error" class="error">{{ error }}</p>
    <div ref="el" class="topo-wrap" />
  </div>
</template>
