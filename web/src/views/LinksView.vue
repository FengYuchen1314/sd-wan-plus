<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api'

const links = ref<any[]>([])
const nodes = ref<any[]>([])
const form = ref({ node_a: '', node_b: '', initiator_node_id: '', listener_address: '', enabled: true })
const error = ref('')

async function load() {
  links.value = await api.links()
  const n = await api.nodes()
  nodes.value = n.nodes
  if (!form.value.node_a && nodes.value[0]) form.value.node_a = nodes.value[0].id
  if (!form.value.node_b && nodes.value[1]) form.value.node_b = nodes.value[1].id
  form.value.initiator_node_id = form.value.node_a
}

async function create() {
  try {
    await api.createLink(form.value)
    await load()
  } catch (e: any) {
    error.value = e.message
  }
}

async function toggle(id: string, enabled: boolean) {
  await api.updateLink(id, { enabled: !enabled })
  await load()
}

async function remove(id: string) {
  if (!confirm('删除链路？')) return
  await api.deleteLink(id)
  await load()
}

function name(id: string) {
  return nodes.value.find((n) => n.id === id)?.display_name || id.slice(0, 8)
}

onMounted(load)
</script>

<template>
  <div>
    <h1 class="page-title">链路</h1>
    <p class="page-sub">每条链路独立 WireGuard 接口；主动端发起握手，被动端学习 Endpoint</p>
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
        <input v-model="form.listener_address" placeholder="IP 或域名" />
      </div>
      <button @click="create">建立链路</button>
      <p v-if="error" class="error">{{ error }}</p>
    </div>
    <div class="card" style="overflow:auto">
      <table class="data">
        <thead><tr><th>A</th><th>B</th><th>监听</th><th>状态</th><th></th></tr></thead>
        <tbody>
          <tr v-for="l in links" :key="l.id">
            <td>{{ name(l.node_a) }}</td>
            <td>{{ name(l.node_b) }}</td>
            <td class="mono">{{ l.listener_address }}:{{ l.listener_port }}</td>
            <td><span class="badge" :class="l.enabled ? 'ok' : 'warn'">{{ l.status }}</span></td>
            <td class="row-actions">
              <button class="secondary" @click="toggle(l.id, l.enabled)">{{ l.enabled ? '禁用' : '启用' }}</button>
              <button class="danger" @click="remove(l.id)">删除</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
