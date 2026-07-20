<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api'

const nodes = ref<any[]>([])
const tokens = ref<any[]>([])
const parentId = ref('')
const parentAddress = ref('')
const nodeName = ref('')
const expires = ref(60)
const command = ref('')
const error = ref('')

async function load() {
  const n = await api.nodes()
  nodes.value = n.nodes
  if (!parentId.value && nodes.value.length) parentId.value = nodes.value[0].id
  tokens.value = await api.tokens()
}

async function create() {
  error.value = ''
  try {
    const res: any = await api.createToken({
      parent_node_id: parentId.value,
      suggested_node_name: nodeName.value,
      parent_address: parentAddress.value || undefined,
      expires_minutes: expires.value,
    })
    command.value = res.install_command
    await load()
  } catch (e: any) {
    error.value = e.message
  }
}

async function revoke(id: string) {
  await api.revokeToken(id)
  await load()
}

function copy() {
  navigator.clipboard.writeText(command.value)
}

onMounted(load)
</script>

<template>
  <div>
    <h1 class="page-title">接入新节点</h1>
    <p class="page-sub">选择父节点生成一次性安装命令；新节点全部文件只从父节点拉取，不访问 GitHub / 不必直连控制机</p>
    <div class="card" style="max-width:560px">
      <div class="form-row">
        <label>父节点</label>
        <select v-model="parentId">
          <option v-for="n in nodes" :key="n.id" :value="n.id">{{ n.display_name }}</option>
        </select>
      </div>
      <div class="form-row">
        <label>父节点可达地址（可选）</label>
        <input v-model="parentAddress" placeholder="留空则使用已保存地址" />
      </div>
      <div class="form-row">
        <label>新节点名称</label>
        <input v-model="nodeName" required />
      </div>
      <div class="form-row">
        <label>Token 有效期（分钟）</label>
        <input v-model.number="expires" type="number" min="1" max="1440" />
      </div>
      <button :disabled="!nodeName || !parentId" @click="create">生成安装命令</button>
      <p v-if="error" class="error">{{ error }}</p>
      <div v-if="command" style="margin-top:1rem">
        <label style="color:var(--muted);font-size:0.8rem">安装命令</label>
        <pre class="mono" style="background:var(--bg);padding:0.75rem;border-radius:8px;margin:0.4rem 0;white-space:pre-wrap;word-break:break-all">{{ command }}</pre>
        <button class="secondary" @click="copy">复制</button>
      </div>
    </div>

    <h2 style="margin:1.5rem 0 0.75rem;font-size:1rem">近期 Token</h2>
    <div class="card" style="overflow:auto">
      <table class="data">
        <thead><tr><th>名称</th><th>父节点</th><th>过期</th><th>状态</th><th></th></tr></thead>
        <tbody>
          <tr v-for="t in tokens" :key="t.id">
            <td>{{ t.suggested_node_name }}</td>
            <td class="mono">{{ t.parent_node_id.slice(0,8) }}…</td>
            <td class="mono">{{ t.expires_at }}</td>
            <td>
              <span class="badge" :class="t.revoked || t.used_at ? 'warn' : 'ok'">
                {{ t.revoked ? '已撤销' : t.used_at ? '已使用' : '有效' }}
              </span>
            </td>
            <td><button v-if="!t.revoked && !t.used_at" class="secondary" @click="revoke(t.id)">撤销</button></td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
