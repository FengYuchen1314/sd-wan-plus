<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api'

const jobs = ref<any[]>([])
const targets = ref<any[]>([])
const version = ref('')
const error = ref('')

async function load() {
  jobs.value = await api.updates()
}

async function create() {
  try {
    await api.createUpdate({ target_version: version.value })
    version.value = ''
    await load()
  } catch (e: any) {
    error.value = e.message
  }
}

async function start(id: string) {
  await api.startUpdate(id)
  targets.value = await api.updateTargets(id)
  await load()
}

async function rollback(id: string) {
  await api.rollbackUpdate(id)
  await load()
}

async function show(id: string) {
  targets.value = await api.updateTargets(id)
}

onMounted(load)
</script>

<template>
  <div>
    <h1 class="page-title">更新中心</h1>
    <p class="page-sub">制品沿控制树从父节点预分发（节点不访问 GitHub）；叶子优先安装，控制机最后更新</p>
    <div class="row-actions" style="margin-bottom:1rem">
      <input v-model="version" placeholder="目标版本 e.g. 0.2.0" style="max-width:220px" />
      <button :disabled="!version" @click="create">创建更新任务</button>
    </div>
    <p v-if="error" class="error">{{ error }}</p>
    <div class="card" style="overflow:auto;margin-bottom:1rem">
      <table class="data">
        <thead><tr><th>版本</th><th>状态</th><th>创建时间</th><th></th></tr></thead>
        <tbody>
          <tr v-for="j in jobs" :key="j.id">
            <td class="mono">{{ j.target_version }}</td>
            <td><span class="badge ok">{{ j.status }}</span></td>
            <td class="mono">{{ j.created_at }}</td>
            <td class="row-actions">
              <button class="secondary" @click="show(j.id)">目标</button>
              <button @click="start(j.id)">启动</button>
              <button class="danger" @click="rollback(j.id)">回滚</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <div v-if="targets.length" class="card" style="overflow:auto">
      <table class="data">
        <thead><tr><th>节点</th><th>深度</th><th>状态</th><th>错误</th></tr></thead>
        <tbody>
          <tr v-for="t in targets" :key="t.id">
            <td class="mono">{{ t.node_id }}</td>
            <td>{{ t.depth }}</td>
            <td><span class="badge">{{ t.status }}</span></td>
            <td>{{ t.error_message || '—' }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
