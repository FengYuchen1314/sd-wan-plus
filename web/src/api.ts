const BASE = '';

async function request(path: string, options?: RequestInit) {
  const res = await fetch(`${BASE}${path}`, {
    credentials: 'include',
    headers: { 'Content-Type': 'application/json', ...options?.headers },
    ...options,
  });
  if (res.status === 401) {
    window.dispatchEvent(new CustomEvent('pw:unauthorized'));
    throw new Error('Session expired');
  }
  if (!res.ok) {
    const err = await res.json().catch(() => ({ detail: res.statusText }));
    throw new Error(err.detail || err.error || res.statusText);
  }
  return res.json();
}

export const api = {
  me: () => request('/api/auth/me'),
  login: (password: string) =>
    request('/api/auth/login', { method: 'POST', body: JSON.stringify({ username: 'admin', password }) }),
  logout: () => request('/api/auth/logout', { method: 'POST' }),
  changePassword: (current_password: string, new_password: string) =>
    request('/api/auth/change-password', { method: 'POST', body: JSON.stringify({ current_password, new_password }) }),
  revokeSessions: () => request('/api/auth/revoke-sessions', { method: 'POST' }),
  getSessions: () => request('/api/auth/sessions'),

  getNodes: () => request('/api/nodes'),
  renameNode: (id: string, display_name: string) =>
    request(`/api/nodes/${id}/rename`, { method: 'PUT', body: JSON.stringify({ display_name }) }),
  updateNode: (id: string, data: Record<string, unknown>) =>
    request(`/api/nodes/${id}`, { method: 'PUT', body: JSON.stringify(data) }),

  getLinks: () => request('/api/links'),
  createLink: (data: Record<string, unknown>) =>
    request('/api/links', { method: 'POST', body: JSON.stringify(data) }),
  updateLink: (id: string, data: Record<string, unknown>) =>
    request(`/api/links/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  deleteLink: (id: string) =>
    request(`/api/links/${id}`, { method: 'DELETE' }),

  getPolicies: () => request('/api/policies'),
  createPolicy: (data: Record<string, unknown>) =>
    request('/api/policies', { method: 'POST', body: JSON.stringify(data) }),
  updatePolicy: (id: string, data: Record<string, unknown>) =>
    request(`/api/policies/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  deletePolicy: (id: string) =>
    request(`/api/policies/${id}`, { method: 'DELETE' }),

  previewConfig: () => request('/api/config/preview'),
  publishConfig: (reason?: string) =>
    request('/api/config/publish', { method: 'POST', body: JSON.stringify({ reason: reason || 'Manual publish' }) }),
  getConfigRevisions: () => request('/api/config/revisions'),

  createToken: (data: Record<string, unknown>) =>
    request('/api/enrollment/tokens', { method: 'POST', body: JSON.stringify(data) }),
  getTokens: () => request('/api/enrollment/tokens'),
  revokeToken: (id: string) => request(`/api/enrollment/tokens/${id}/revoke`, { method: 'POST' }),

  getUpdates: () => request('/api/updates'),
  createUpdate: (data: Record<string, unknown>) =>
    request('/api/updates', { method: 'POST', body: JSON.stringify(data) }),
  startUpdate: (id: string) => request(`/api/updates/${id}/start`, { method: 'POST' }),
  rollbackUpdate: (id: string) => request(`/api/updates/${id}/rollback`, { method: 'POST' }),

  getAuditLogs: (limit?: number) => request(`/api/audit-logs?limit=${limit || 200}`),
};
