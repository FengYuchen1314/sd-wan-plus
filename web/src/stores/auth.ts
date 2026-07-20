import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api } from '../api'

export const useAuthStore = defineStore('auth', () => {
  const user = ref<{ id: string; username: string } | null>(null)
  const checked = ref(false)

  async function check() {
    try {
      user.value = await api.me()
    } catch {
      user.value = null
    } finally {
      checked.value = true
    }
  }

  async function login(password: string) {
    await api.login(password)
    user.value = await api.me()
  }

  async function logout() {
    await api.logout()
    user.value = null
  }

  return { user, checked, check, login, logout }
})
