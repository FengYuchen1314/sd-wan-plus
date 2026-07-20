import { useState } from 'react';
import Dashboard from './pages/Dashboard';
import Topology from './pages/Topology';
import NodesPage from './pages/Nodes';
import Enrollment from './pages/Enrollment';
import Policies from './pages/Policies';
import Updates from './pages/Updates';

function Login({ onLogin }: { onLogin: (t: string) => void }) {
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  const submit = async () => {
    setLoading(true); setError('');
    try {
      const res = await fetch('/api/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username: 'admin', password }),
      });
      const data = await res.json();
      if (res.ok) { onLogin(data.token); }
      else { setError(data.error || '登录失败'); }
    } catch {
      setError('无法连接控制机');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div style={{ height: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', background: 'linear-gradient(135deg, #0f172a, #1e293b)' }}>
      <div style={{ background: '#fff', borderRadius: 12, padding: 40, width: 380, boxShadow: '0 20px 60px rgba(0,0,0,0.3)' }}>
        <h1 style={{ textAlign: 'center', marginBottom: 4 }}>PathWeaver</h1>
        <p style={{ textAlign: 'center', color: '#64748b', marginBottom: 24, fontSize: 14 }}>SD-WAN 组网控制平台</p>
        <label style={{ fontSize: 13, color: '#475569', marginBottom: 4, display: 'block' }}>管理员密码</label>
        <input type="password" value={password} onChange={e => setPassword(e.target.value)}
          onKeyDown={e => e.key === 'Enter' && submit()} placeholder="请输入密码" autoFocus
          style={{ width: '100%', padding: '10px 12px', border: '1px solid #cbd5e1', borderRadius: 8, fontSize: 14, marginBottom: 16, boxSizing: 'border-box' }} />
        {error && <p style={{ color: '#ef4444', fontSize: 13, marginBottom: 12 }}>{error}</p>}
        <button onClick={submit} disabled={loading} style={{
          width: '100%', padding: '10px', background: '#0f172a', color: '#fff', border: 'none',
          borderRadius: 8, fontSize: 14, cursor: 'pointer', opacity: loading ? 0.7 : 1,
        }}>{loading ? '连接中...' : '登录'}</button>
      </div>
    </div>
  );
}

const PAGES: Record<string, React.FC> = {
  dashboard: Dashboard,
  topology: Topology,
  nodes: NodesPage,
  enrollment: Enrollment,
  policies: Policies,
  updates: Updates,
};

const NAV = [
  { key: 'dashboard', label: '仪表盘' },
  { key: 'topology', label: '拓扑图' },
  { key: 'nodes', label: '节点管理' },
  { key: 'enrollment', label: '接入新节点' },
  { key: 'policies', label: '路径策略' },
  { key: 'updates', label: '更新中心' },
];

export default function App() {
  const [token, setToken] = useState(localStorage.getItem('pw_token'));
  const [page, setPage] = useState('dashboard');

  if (!token) {
    return <Login onLogin={(t: string) => { localStorage.setItem('pw_token', t); setToken(t); }} />;
  }

  const logout = () => { localStorage.removeItem('pw_token'); setToken(null); };
  const Page = PAGES[page] || (() => <div>404</div>);

  return (
    <div style={{ display: 'flex', height: '100vh' }}>
      <aside style={{ width: 220, background: '#0f172a', color: '#e2e8f0', display: 'flex', flexDirection: 'column', padding: '16px 0' }}>
        <div style={{ padding: '0 16px 24px', fontSize: 18, fontWeight: 700, color: '#38bdf8' }}>PathWeaver</div>
        {NAV.map(({ key, label }) => (
          <div key={key} onClick={() => setPage(key)} style={{
            display: 'flex', alignItems: 'center', padding: '10px 20px', cursor: 'pointer',
            color: page === key ? '#38bdf8' : '#94a3b8', fontSize: 14,
            background: page === key ? 'rgba(56,189,248,0.1)' : 'transparent',
            borderLeft: page === key ? '3px solid #38bdf8' : '3px solid transparent',
          }}>{label}</div>
        ))}
        <div style={{ flex: 1 }} />
        <button onClick={logout} style={{
          margin: '0 16px', padding: '8px 12px', background: 'transparent',
          border: '1px solid #334155', borderRadius: 6, color: '#94a3b8', cursor: 'pointer', fontSize: 13,
        }}>退出登录</button>
      </aside>
      <main style={{ flex: 1, overflow: 'auto', background: '#f1f5f9', padding: 24 }}>
        <Page />
      </main>
    </div>
  );
}
