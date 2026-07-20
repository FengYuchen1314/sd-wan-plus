<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { api } from '../api'

const links = ref<any[]>([])
const nodes = ref<any[]>([])
const addrByNode = ref<Record<string, any[]>>({})
const form = ref({
  node_a: '',
  node_b: '',
  initiator_node_id: '',
  listener_address: '',
  listener_port: 14303 as number | null,
  enabled: true,
  bidirectional: false,
})
const error = ref('')

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

function nodeIsPublic(id: string) {
  const addrs = addrByNode.value[id] || []
  if (!addrs.length) return false
  for (const a of addrs) {
    const t = (a.address_type || '').toLowerCase()
    if (t === 'lan') continue
    const host = String(a.address || '').trim()
    if (!host) continue
    if (t === 'public' || !isPrivateIPv4(host)) return true
  }
  return false
}

const listenerId = computed(() => {
  if (!form.value.initiator_node_id) return form.value.node_b
  return form.value.initiator_node_id === form.value.node_a ? form.value.node_b : form.value.node_a
})

const showListenerPort = computed(() => {
  const id = listenerId.value
  return !!id && !nodeIsPublic(id)
})

function syncListenerDefaults() {
  const id = listenerId.value
  const n = nodes.value.find((x) => x.id === id)
  if (n) {
    form.value.listener_port = n.wg_port_range_start || 14303
  }
  const addrs = addrByNode.value[id] || []
  const pub = addrs.find((a) => (a.address_type || '').toLowerCase() === 'public')
  const primary = addrs.find((a) => a.is_primary)
  const suggest = pub?.address || primary?.address || addrs[0]?.address || ''
  if (suggest && !form.value.listener_address) {
    form.value.listener_address = suggest
  } else if (suggest) {
    form.value.listener_address = suggest
  }
}

async function load() {
  links.value = await api.links()
  const n = await api.nodes()
  nodes.value = n.nodes
  if (!form.value.node_a && nodes.value[0]) form.value.node_a = nodes.value[0].id
  if (!form.value.node_b && nodes.value[1]) form.value.node_b = nodes.value[1].id
  form.value.initiator_node_id = form.value.node_a

  const map: Record<string, any[]> = {}
  await Promise.all(nodes.value.map(async (node) => {
    try {
      map[node.id] = await api.nodeAddresses(node.id)
    } catch {
      map[node.id] = []
    }
  }))
  addrByNode.value = map
  syncListenerDefaults()
}

async function create() {
  error.value = ''
  try {
    const body: any = {
      node_a: form.value.node_a,
      node_b: form.value.node_b,
      initiator_node_id: form.value.initiator_node_id,
      listener_address: form.value.listener_address,
      enabled: form.value.enabled,
      bidirectional: form.value.bidirectional,
    }
    if (showListenerPort.value) {
      body.listener_port = Number(form.value.listener_port)
    }
    await api.createLink(body)
    form.value.bidirectional = false
    await load()
  } catch (e: any) {
    error.value = e.message
  }
}

async function toggle(id: string, enabled: boolean) {
  await api.updateLink(id, { enabled: !enabled })
  await load()
}

async function setBidirectional(id: string, bidirectional: boolean) {
  try {
    await api.updateLink(id, { bidirectional: !bidirectional })
    await load()
  } catch (e: any) {
    error.value = e.message
  }
}

async function remove(id: string) {
  if (!confirm('删除链路？')) return
  await api.deleteLink(id)
  await load()
}

function name(id: string) {
  return nodes.value.find((n) => n.id === id)?.display_name || id.slice(0, 8)
}

watch([() => form.value.node_a, () => form.value.node_b, () => form.value.initiator_node_id], () => {
  syncListenerDefaults()
})

onMounted(load)
</script>

<template>
  <div>
    <h1 class="page-title">链路</h1>
    <p class="page-sub">默认单向发起握手（含双公网）；仅手动勾选双向互拨时两侧互拨。内网连公网请让内网节点作主动端。被动端为内网时可自定义监听端口（端口映射）。</p>
    <div class="card" style="max-width:560px;margin-bottom:1.25rem">
      <div class="form-row"><label>节点 A</label>
        <select v-model="form.node_a"><option v-for="n in nodes" :key="n.id" :value="n.id">{{ n.display_name }}</option></select>
      </div>
      <div class="form-row"><label>节点 B</label>
        <select v-model="form.node_b"><option v-for="n in nodes" :key="n.id" :value="n.id">{{ n.display_name }}</option></select>
      </div>
      <div class="form-row"><label>主动发起端</label>
        <select v-model="form.initiator_node_id">
          <option :value="form.node_a">{{ name(form.node_a) }} (A)</option>
          <option :value="form.node_b">{{ name(form.node_b) }} (B)</option>
        </select>
      </div>
      <div class="form-row"><label>被访问端地址</label>
        <input v-model="form.listener_address" placeholder="被动端公网 IP 或域名" />
      </div>
      <div v-if="showListenerPort" class="form-row">
        <label>被访问端端口</label>
        <input v-model.number="form.listener_port" type="number" min="1" max="65535" />
        <p class="page-sub" style="margin:0.35rem 0 0">默认取节点库中端口；有端口映射时可改</p>
      </div>
      <div class="form-row"><label>
        <input type="checkbox" v-model="form.bidirectional" />
        双向互拨（仅双公网；默认不要勾）
      </label></div>
      <button @click="create">建立链路</button>
      <p v-if="error" class="error">{{ error }}</p>
    </div>
    <div class="card" style="overflow:auto">
      <table class="data">
        <thead><tr><th>A</th><th>B</th><th>监听</th><th>模式</th><th>状态</th><th></th></tr></thead>
        <tbody>
          <tr v-for="l in links" :key="l.id">
            <td>{{ name(l.node_a) }}</td>
            <td>{{ name(l.node_b) }}</td>
            <td class="mono">{{ l.listener_address }}:{{ l.listener_port }}</td>
            <td>{{ l.bidirectional ? '双向' : '单向' }}</td>
            <td><span class="badge" :class="l.enabled ? 'ok' : 'warn'">{{ l.status }}</span></td>
            <td class="row-actions">
              <button class="secondary" @click="toggle(l.id, l.enabled)">{{ l.enabled ? '禁用' : '启用' }}</button>
              <button class="secondary" @click="setBidirectional(l.id, !!l.bidirectional)">{{ l.bidirectional ? '改单向' : '改双向' }}</button>
              <button class="danger" @click="remove(l.id)">删除</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
