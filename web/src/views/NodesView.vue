<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api'

const nodes = ref<any[]>([])
const statuses = ref<any[]>([])
const renameId = ref('')
const renameVal = ref('')
const error = ref('')
const msg = ref('')

async function load() {
  const res = await api.nodes()
  nodes.value = res.nodes
  statuses.value = res.statuses
}

function statusOf(id: string) {
  return statuses.value.find((s) => s.node_id === id)
}

async function doRename(id: string) {
  try {
    await api.renameNode(id, renameVal.value)
    msg.value = '已重命名'
    renameId.value = ''
    await load()
  } catch (e: any) {
    error.value = e.message
  }
}

onMounted(load)
</script>

<template>
  <div>
    <h1 class="page-title">节点</h1>
    <p class="page-sub">身份 UUID 与显示名分离；可查看 Overlay、父节点与多维状态</p>
    <p v-if="error" class="error">{{ error }}</p>
    <p v-if="msg" class="success">{{ msg }}</p>
    <div class="card" style="overflow:auto">
      <table class="data">
        <thead>
          <tr>
            <th>显示名</th><th>Overlay</th><th>Agent</th><th>配置</th><th>版本</th><th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="n in nodes" :key="n.id">
            <td>
              <div>{{ n.display_name }}</div>
              <div class="mono" style="color:var(--muted);font-size:0.72rem">{{ n.id.slice(0, 8) }}…</div>
              <span v-if="n.is_controller" class="badge ok">控制机</span>
            </td>
            <td class="mono">{{ n.overlay_ipv4 }}</td>
            <td>
              <span class="badge" :class="statusOf(n.id)?.agent_online ? 'ok' : 'err'">
                {{ statusOf(n.id)?.agent_online ? '在线' : '离线' }}
              </span>
            </td>
            <td>
              <span class="badge" :class="statusOf(n.id)?.config_consistent ? 'ok' : 'warn'">
                {{ statusOf(n.id)?.config_consistent ? '一致' : '漂移' }}
              </span>
            </td>
            <td class="mono">{{ n.agent_version || '—' }}</td>
            <td>
              <template v-if="renameId === n.id">
                <input v-model="renameVal" style="width:140px;margin-right:0.35rem" />
                <button @click="doRename(n.id)">保存</button>
              </template>
              <button v-else class="secondary" @click="renameId = n.id; renameVal = n.display_name">重命名</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
