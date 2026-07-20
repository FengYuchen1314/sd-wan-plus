const BASE = ''

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(BASE + path, {
    credentials: 'include',
    headers: { 'Content-Type': 'application/json', ...(init?.headers || {}) },
    ...init,
  })
  if (res.status === 401) {
    window.dispatchEvent(new Event('pw:unauthorized'))
    throw new Error('unauthorized')
  }
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.message || body.code || res.statusText)
  }
  if (res.status === 204) return undefined as T
  return res.json()
}

export const api = {
  health: () => request<{ status: string; version: string }>('/api/health'),
  login: (password: string) => request('/api/auth/login', { method: 'POST', body: JSON.stringify({ password }) }),
  logout: () => request('/api/auth/logout', { method: 'POST' }),
  me: () => request<{ id: string; username: string }>('/api/auth/me'),
  changePassword: (current_password: string, new_password: string) =>
    request('/api/auth/change-password', { method: 'POST', body: JSON.stringify({ current_password, new_password }) }),
  revokeSessions: () => request('/api/auth/revoke-sessions', { method: 'POST' }),

  dashboard: () => request<Record<string, number | string>>('/api/dashboard'),
  nodes: () => request<{ nodes: any[]; statuses: any[] }>('/api/nodes'),
  renameNode: (id: string, display_name: string) =>
    request(`/api/nodes/${id}/rename`, { method: 'PUT', body: JSON.stringify({ display_name }) }),
  updateNode: (id: string, body: any) => request(`/api/nodes/${id}`, { method: 'PUT', body: JSON.stringify(body) }),
  nodeAddresses: (id: string) => request<any[]>(`/api/nodes/${id}/addresses`),
  addAddress: (id: string, body: any) =>
    request(`/api/nodes/${id}/addresses`, { method: 'POST', body: JSON.stringify(body) }),

  topology: () => request<any>('/api/topology'),
  links: () => request<any[]>('/api/links'),
  createLink: (body: any) => request('/api/links', { method: 'POST', body: JSON.stringify(body) }),
  updateLink: (id: string, body: any) => request(`/api/links/${id}`, { method: 'PUT', body: JSON.stringify(body) }),
  deleteLink: (id: string) => request(`/api/links/${id}`, { method: 'DELETE' }),

  policies: () => request<any[]>('/api/policies'),
  getPolicy: (id: string) => request<any>(`/api/policies/${id}`),
  createPolicy: (body: any) => request('/api/policies', { method: 'POST', body: JSON.stringify(body) }),
  deletePolicy: (id: string) => request(`/api/policies/${id}`, { method: 'DELETE' }),

  tokens: () => request<any[]>('/api/enrollment/tokens'),
  createToken: (body: any) => request('/api/enrollment/tokens', { method: 'POST', body: JSON.stringify(body) }),
  revokeToken: (id: string) => request(`/api/enrollment/tokens/${id}/revoke`, { method: 'POST' }),

  previewConfig: () => request<any>('/api/config/preview'),
  publishConfig: (reason: string) =>
    request('/api/config/publish', { method: 'POST', body: JSON.stringify({ reason }) }),
  revisions: () => request<any[]>('/api/config/revisions'),
  revisionNodes: (id: string) => request<any[]>(`/api/config/revisions/${id}/nodes`),

  updates: () => request<any[]>('/api/updates'),
  updatesLatest: () => request<any>('/api/updates/latest'),
  updatesOverview: () => request<any>('/api/updates/overview'),
  pullLatest: (auto_start = true) =>
    request('/api/updates/pull-latest', { method: 'POST', body: JSON.stringify({ auto_start }) }),
  createUpdate: (body: any) => request('/api/updates', { method: 'POST', body: JSON.stringify(body) }),
  startUpdate: (id: string) => request(`/api/updates/${id}/start`, { method: 'POST' }),
  rollbackUpdate: (id: string) => request(`/api/updates/${id}/rollback`, { method: 'POST' }),
  updateTargets: (id: string) => request<any[]>(`/api/updates/${id}/targets`),

  auditLogs: () => request<any[]>('/api/audit-logs'),
}

export function connectWS(onMessage: (msg: any) => void) {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws'
  const ws = new WebSocket(`${proto}://${location.host}/ws`)
  ws.onmessage = (ev) => {
    try {
      onMessage(JSON.parse(ev.data))
    } catch {
      /* ignore */
    }
  }
  return ws
}
