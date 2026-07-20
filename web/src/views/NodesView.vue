<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api'

const nodes = ref<any[]>([])
const statuses = ref<any[]>([])
const renameId = ref('')
const renameVal = ref('')
const error = ref('')
const msg = ref('')
const deleting = ref(false)

const confirmOpen = ref(false)
const confirmNode = ref<any>(null)
const impact = ref<any>(null)
const impactLoading = ref(false)

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

async function openDelete(n: any) {
  error.value = ''
  msg.value = ''
  confirmNode.value = n
  impact.value = null
  confirmOpen.value = true
  impactLoading.value = true
  try {
    impact.value = await api.deleteNodeImpact(n.id)
  } catch (e: any) {
    error.value = e.message
    confirmOpen.value = false
  } finally {
    impactLoading.value = false
  }
}

function closeDelete() {
  confirmOpen.value = false
  confirmNode.value = null
  impact.value = null
}

async function confirmDelete() {
  if (!confirmNode.value || !impact.value?.can_delete) return
  deleting.value = true
  error.value = ''
  try {
    const res: any = await api.deleteNode(confirmNode.value.id)
    msg.value = res.note || `已删除「${impact.value.display_name}」并发布配置`
    closeDelete()
    await load()
  } catch (e: any) {
    error.value = e.message
  } finally {
    deleting.value = false
  }
}

onMounted(load)
</script>

<template>
  <div>
    <h1 class="page-title">节点</h1>
    <p class="page-sub">身份 UUID 与显示名分离；删除前请先在面板移除，再在设备上卸载</p>
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
            <td style="white-space:nowrap">
              <template v-if="renameId === n.id">
                <input v-model="renameVal" style="width:140px;margin-right:0.35rem" />
                <button @click="doRename(n.id)">保存</button>
              </template>
              <template v-else>
                <button class="secondary" @click="renameId = n.id; renameVal = n.display_name">重命名</button>
                <button
                  class="danger"
                  style="margin-left:0.35rem"
                  :disabled="n.is_controller"
                  :title="n.is_controller ? '控制机不可删除' : '删除节点'"
                  @click="openDelete(n)"
                >{{ n.is_controller ? '控制机不可删' : '删除' }}</button>
              </template>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div v-if="confirmOpen" class="modal-backdrop" @click.self="closeDelete">
      <div class="modal card" role="dialog" aria-modal="true">
        <h2 style="margin-bottom:0.5rem">删除节点</h2>
        <p v-if="impactLoading" class="page-sub">正在计算影响…</p>
        <template v-else-if="impact">
          <p style="margin-bottom:0.75rem">{{ impact.warning }}</p>
          <ul class="page-sub" style="margin:0 0 1rem 1.1rem">
            <li>相邻链路：{{ impact.link_count }}</li>
            <li>控制树下级：{{ impact.child_count }}</li>
          </ul>
          <p v-if="!impact.can_delete" class="error" style="margin-bottom:0.75rem">{{ impact.block_reason }}</p>
          <p v-if="impact.can_delete" class="page-sub" style="margin-bottom:1rem">
            删除后请在该设备执行卸载（可保留或 purge 数据）。
          </p>
          <div class="modal-actions">
            <button class="danger" :disabled="!impact.can_delete || deleting" @click="confirmDelete">
              {{ deleting ? '删除中…' : '确认删除' }}
            </button>
            <span style="flex:1"></span>
            <button class="secondary" :disabled="deleting" @click="closeDelete">取消</button>
          </div>
        </template>
      </div>
    </div>
  </div>
</template>

<style scoped>
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
}
.modal-actions {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}
</style>
