<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api'

const policies = ref<any[]>([])
const nodes = ref<any[]>([])
const hops = ref<string[]>([])
const form = ref({
  name: '',
  priority: 100,
  source_cidr: '',
  destination_cidr: '0.0.0.0/0',
  protocol: 'Any',
  port: undefined as number | undefined,
  egress_nat: false,
})
const error = ref('')
const detail = ref<any>(null)

async function load() {
  policies.value = await api.policies()
  const n = await api.nodes()
  nodes.value = n.nodes
}

function addHop(id: string) {
  if (!hops.value.includes(id)) hops.value.push(id)
}

function clearHops() { hops.value = [] }

async function create() {
  error.value = ''
  try {
    await api.createPolicy({
      name: form.value.name,
      priority: form.value.priority,
      enabled: true,
      hops: hops.value,
      match: {
        source_cidr: form.value.source_cidr || undefined,
        destination_cidr: form.value.destination_cidr || undefined,
        protocol: form.value.protocol,
        destination_port_start: form.value.port,
        destination_port_end: form.value.port,
      },
      egress_nat: form.value.egress_nat,
      return_path_type: 'Symmetric',
    })
    hops.value = []
    form.value.name = ''
    await load()
  } catch (e: any) {
    error.value = e.message
  }
}

async function show(id: string) {
  detail.value = await api.getPolicy(id)
}

async function remove(id: string) {
  await api.deletePolicy(id)
  await load()
}

function name(id: string) {
  return nodes.value.find((n) => n.id === id)?.display_name || id.slice(0, 8)
}

onMounted(load)
</script>

<template>
  <div>
    <h1 class="page-title">路径策略</h1>
    <p class="page-sub">按源/目标/协议/端口匹配，沿显式节点序列转发；相邻节点须已有链路</p>
    <div class="card" style="max-width:640px;margin-bottom:1.25rem">
      <div class="form-row"><label>名称</label><input v-model="form.name" /></div>
      <div class="form-row"><label>优先级（越小越高）</label><input v-model.number="form.priority" type="number" /></div>
      <div class="form-row"><label>源 CIDR</label><input v-model="form.source_cidr" placeholder="可选" /></div>
      <div class="form-row"><label>目标 CIDR</label><input v-model="form.destination_cidr" /></div>
      <div class="form-row"><label>协议</label>
        <select v-model="form.protocol"><option>Any</option><option>TCP</option><option>UDP</option><option>ICMP</option></select>
      </div>
      <div class="form-row"><label>目标端口</label><input v-model.number="form.port" type="number" placeholder="可选" /></div>
      <div class="form-row"><label>路径节点（按序点击）</label>
        <div class="row-actions">
          <button v-for="n in nodes" :key="n.id" class="secondary" type="button" @click="addHop(n.id)">{{ n.display_name }}</button>
          <button class="secondary" type="button" @click="clearHops">清空</button>
        </div>
        <div class="mono" style="margin-top:0.5rem">{{ hops.map(name).join(' → ') || '未选择' }}</div>
      </div>
      <label style="display:flex;gap:0.5rem;align-items:center;margin-bottom:0.75rem">
        <input v-model="form.egress_nat" type="checkbox" style="width:auto" /> 出口 NAT
      </label>
      <button :disabled="hops.length < 2 || !form.name" @click="create">保存策略</button>
      <p v-if="error" class="error">{{ error }}</p>
    </div>
    <div class="card" style="overflow:auto">
      <table class="data">
        <thead><tr><th>名称</th><th>优先级</th><th>状态</th><th></th></tr></thead>
        <tbody>
          <tr v-for="p in policies" :key="p.id">
            <td>{{ p.name }}</td>
            <td>{{ p.priority }}</td>
            <td><span class="badge" :class="p.enabled ? 'ok' : 'warn'">{{ p.enabled ? '启用' : '禁用' }}</span></td>
            <td class="row-actions">
              <button class="secondary" @click="show(p.id)">详情</button>
              <button class="danger" @click="remove(p.id)">删除</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <pre v-if="detail" class="card mono" style="margin-top:1rem;overflow:auto">{{ JSON.stringify(detail, null, 2) }}</pre>
  </div>
</template>
