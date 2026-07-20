import { useState } from 'react';

export default function Login({ onLogin }: { onLogin: (t: string) => void }) {
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
      if (res.ok) {
        onLogin(data.token);
      } else {
        setError(data.error || '登录失败');
      }
    } catch {
      setError('无法连接到控制机');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div style={{
      height: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center',
      background: 'linear-gradient(135deg, #0f172a 0%, #1e293b 100%)',
    }}>
      <div style={{
        background: '#fff', borderRadius: 12, padding: 40, width: 380,
        boxShadow: '0 20px 60px rgba(0,0,0,0.3)',
      }}>
        <h1 style={{ textAlign: 'center', color: '#0f172a', marginBottom: 4 }}>PathWeaver</h1>
        <p style={{ textAlign: 'center', color: '#64748b', marginBottom: 24, fontSize: 14 }}>
          SD-WAN 组网控制平台
        </p>
        <label style={{ fontSize: 13, color: '#475569', marginBottom: 4, display: 'block' }}>管理员密码</label>
        <input
          type="password"
          value={password}
          onChange={e => setPassword(e.target.value)}
          onKeyDown={e => e.key === 'Enter' && submit()}
          placeholder="请输入密码"
          style={{
            width: '100%', padding: '10px 12px', border: '1px solid #cbd5e1',
            borderRadius: 8, fontSize: 14, marginBottom: 16, boxSizing: 'border-box',
          }}
          autoFocus
        />
        {error && <p style={{ color: '#ef4444', fontSize: 13, marginBottom: 12 }}>{error}</p>}
        <button
          onClick={submit}
          disabled={loading}
          style={{
            width: '100%', padding: '10px', background: '#0f172a', color: '#fff',
            border: 'none', borderRadius: 8, fontSize: 14, cursor: 'pointer',
            opacity: loading ? 0.7 : 1,
          }}
        >
          {loading ? '连接中...' : '登录'}
        </button>
      </div>
    </div>
  );
}
