import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', name: 'login', component: () => import('../views/LoginView.vue'), meta: { public: true } },
    {
      path: '/',
      component: () => import('../layouts/AppLayout.vue'),
      children: [
        { path: '', name: 'dashboard', component: () => import('../views/DashboardView.vue') },
        { path: 'nodes', name: 'nodes', component: () => import('../views/NodesView.vue') },
        { path: 'topology', name: 'topology', component: () => import('../views/TopologyView.vue') },
        { path: 'enrollment', name: 'enrollment', component: () => import('../views/EnrollmentView.vue') },
        { path: 'links', name: 'links', component: () => import('../views/LinksView.vue') },
        { path: 'policies', name: 'policies', component: () => import('../views/PoliciesView.vue') },
        { path: 'publish', name: 'publish', component: () => import('../views/PublishView.vue') },
        { path: 'updates', name: 'updates', component: () => import('../views/UpdatesView.vue') },
        { path: 'settings', name: 'settings', component: () => import('../views/SettingsView.vue') },
      ],
    },
  ],
})

router.beforeEach(async (to) => {
  if (to.meta.public) return true
  const auth = useAuthStore()
  if (!auth.checked) await auth.check()
  if (!auth.user) return { name: 'login', query: { redirect: to.fullPath } }
  return true
})

export default router
