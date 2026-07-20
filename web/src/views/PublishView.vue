<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api'

const preview = ref<any>(null)
const revisions = ref<any[]>([])
const rollouts = ref<any[]>([])
const reason = ref('manual publish')
const error = ref('')
const msg = ref('')

async function load() {
  revisions.value = await api.revisions()
}

async function doPreview() {
  try {
    preview.value = await api.previewConfig()
  } catch (e: any) {
    error.value = e.message
  }
}

async function publish() {
  try {
    const res: any = await api.publishConfig(reason.value)
    msg.value = `已发布 generation=${res.revision?.generation}`
    await load()
    if (res.revision?.id) rollouts.value = await api.revisionNodes(res.revision.id)
  } catch (e: any) {
    error.value = e.message
  }
}

async function showRollout(id: string) {
  rollouts.value = await api.revisionNodes(id)
}

onMounted(load)
</script>

<template>
  <div>
    <h1 class="page-title">配置发布</h1>
    <p class="page-sub">Prepare → Activate → Verify；失败可回滚到 Last Known Good</p>
    <div class="row-actions" style="margin-bottom:1rem">
      <input v-model="reason" style="max-width:280px" placeholder="发布原因" />
      <button class="secondary" @click="doPreview">配置预览</button>
      <button @click="publish">发布</button>
    </div>
    <p v-if="error" class="error">{{ error }}</p>
    <p v-if="msg" class="success">{{ msg }}</p>
    <pre v-if="preview" class="card mono" style="max-height:320px;overflow:auto;margin-bottom:1rem">{{ JSON.stringify(preview, null, 2) }}</pre>
    <div class="card" style="overflow:auto;margin-bottom:1rem">
      <table class="data">
        <thead><tr><th>Generation</th><th>原因</th><th>状态</th><th>时间</th><th></th></tr></thead>
        <tbody>
          <tr v-for="r in revisions" :key="r.id">
            <td class="mono">{{ r.generation }}</td>
            <td>{{ r.reason }}</td>
            <td><span class="badge ok">{{ r.status }}</span></td>
            <td class="mono">{{ r.created_at }}</td>
            <td><button class="secondary" @click="showRollout(r.id)">节点状态</button></td>
          </tr>
        </tbody>
      </table>
    </div>
    <div v-if="rollouts.length" class="card" style="overflow:auto">
      <table class="data">
        <thead><tr><th>节点</th><th>状态</th><th>错误</th></tr></thead>
        <tbody>
          <tr v-for="n in rollouts" :key="n.id">
            <td class="mono">{{ n.node_id }}</td>
            <td><span class="badge">{{ n.status }}</span></td>
            <td>{{ n.error_message || '—' }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
