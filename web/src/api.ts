const BASE = '';

async function request(path: string, options?: RequestInit) {
  const res = await fetch(`${BASE}${path}`, {
    headers: { 'Content-Type': 'application/json', ...options?.headers },
    ...options,
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err.error || res.statusText);
  }
  return res.json();
}

export const api = {
  login: (username: string, password: string) =>
    request('/api/auth/login', { method: 'POST', body: JSON.stringify({ username, password }) }),

  getNodes: () => request('/api/nodes'),
  renameNode: (id: string, display_name: string) =>
    request(`/api/nodes/${id}/rename`, { method: 'PUT', body: JSON.stringify({ display_name }) }),

  getLinks: () => request('/api/links'),
  createLink: (data: Record<string, unknown>) =>
    request('/api/links', { method: 'POST', body: JSON.stringify(data) }),
  deleteLink: (id: string) =>
    request(`/api/links/${id}`, { method: 'DELETE' }),

  getPolicies: () => request('/api/policies'),
  createPolicy: (data: Record<string, unknown>) =>
    request('/api/policies', { method: 'POST', body: JSON.stringify(data) }),

  previewConfig: () => request('/api/config/preview'),

  createToken: (data: Record<string, unknown>) =>
    request('/api/enrollment/tokens', { method: 'POST', body: JSON.stringify(data) }),
  getInstallCommand: (token: string) =>
    request(`/api/enrollment/command/${token}?token=${token}`),

  getUpdates: () => request('/api/updates'),
  createUpdate: (data: Record<string, unknown>) =>
    request('/api/updates', { method: 'POST', body: JSON.stringify(data) }),
  startUpdate: (id: string) => request(`/api/updates/${id}/start`, { method: 'POST' }),
  rollbackUpdate: (id: string) => request(`/api/updates/${id}/rollback`, { method: 'POST' }),
};
