<script setup lang="ts">
import { ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const password = ref('')
const error = ref('')
const loading = ref(false)
const auth = useAuthStore()
const router = useRouter()
const route = useRoute()

async function submit() {
  error.value = ''
  loading.value = true
  try {
    await auth.login(password.value)
    const redirect = (route.query.redirect as string) || '/'
    router.replace(redirect)
  } catch (e: any) {
    error.value = e.message || '登录失败'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="login">
    <div class="panel card">
      <div class="logo">PathWeaver</div>
      <p class="hint">自托管 SD-WAN 控制台 · 管理员登录</p>
      <form @submit.prevent="submit">
        <div class="form-row">
          <label>管理员密码</label>
          <input v-model="password" type="password" autocomplete="current-password" autofocus />
        </div>
        <button type="submit" :disabled="loading || !password" style="width:100%">
          {{ loading ? '登录中…' : '登录' }}
        </button>
        <p v-if="error" class="error">{{ error }}</p>
      </form>
    </div>
  </div>
</template>

<style scoped>
.login {
  min-height: 100%;
  display: grid; place-items: center;
  background:
    radial-gradient(ellipse at 50% -10%, #1a3a40 0%, transparent 45%),
    radial-gradient(ellipse at 90% 80%, #132840 0%, transparent 40%),
    var(--bg);
  padding: 1.5rem;
}
.panel { width: min(380px, 100%); }
.logo {
  font-size: 1.6rem; font-weight: 750; letter-spacing: -0.04em;
  background: linear-gradient(90deg, #2dd4a8, #4da3ff);
  -webkit-background-clip: text; background-clip: text; color: transparent;
}
.hint { color: var(--muted); margin: 0.4rem 0 1.25rem; }
</style>
