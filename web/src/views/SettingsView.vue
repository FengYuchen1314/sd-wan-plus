<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api'

const current = ref('')
const next = ref('')
const msg = ref('')
const error = ref('')
const logs = ref<any[]>([])

async function load() {
  logs.value = await api.auditLogs()
}

async function changePw() {
  try {
    await api.changePassword(current.value, next.value)
    msg.value = '密码已更新'
    current.value = ''
    next.value = ''
  } catch (e: any) {
    error.value = e.message
  }
}

async function revoke() {
  await api.revokeSessions()
  msg.value = '其他会话已撤销'
}

onMounted(load)
</script>

<template>
  <div>
    <h1 class="page-title">设置</h1>
    <p class="page-sub">单管理员账户 · Session 与审计</p>
    <div class="card" style="max-width:420px;margin-bottom:1.25rem">
      <div class="form-row"><label>当前密码</label><input v-model="current" type="password" /></div>
      <div class="form-row"><label>新密码</label><input v-model="next" type="password" /></div>
      <div class="row-actions">
        <button :disabled="next.length < 8" @click="changePw">修改密码</button>
        <button class="secondary" @click="revoke">撤销其他会话</button>
      </div>
      <p v-if="msg" class="success">{{ msg }}</p>
      <p v-if="error" class="error">{{ error }}</p>
    </div>
    <h2 style="font-size:1rem;margin-bottom:0.75rem">审计日志</h2>
    <div class="card" style="overflow:auto">
      <table class="data">
        <thead><tr><th>时间</th><th>动作</th><th>资源</th><th>IP</th></tr></thead>
        <tbody>
          <tr v-for="l in logs" :key="l.id">
            <td class="mono">{{ l.created_at }}</td>
            <td>{{ l.action }}</td>
            <td>{{ l.resource_type }}</td>
            <td class="mono">{{ l.ip_address }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
