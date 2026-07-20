import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { api } from '../api';

export default function Login() {
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const navigate = useNavigate();

  const submit = async () => {
    if (!password) { setError('请输入密码'); return; }
    setLoading(true); setError('');
    try {
      await api.login(password);
      navigate('/', { replace: true });
    } catch (e: any) {
      setError(e.message || '登录失败');
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
        <h1 style={{ textAlign: 'center', color: '#0f172a', marginBottom: 4, fontSize: 24, fontWeight: 700 }}>
          PathWeaver
        </h1>
        <p style={{ textAlign: 'center', color: '#64748b', marginBottom: 32, fontSize: 14 }}>
          SD-WAN 组网控制平台
        </p>
        <label style={{ fontSize: 13, color: '#475569', marginBottom: 6, display: 'block', fontWeight: 500 }}>
          管理员密码
        </label>
        <input
          type="password"
          value={password}
          onChange={e => setPassword(e.target.value)}
          onKeyDown={e => e.key === 'Enter' && submit()}
          placeholder="请输入密码"
          style={{
            width: '100%', padding: '10px 12px', border: '1px solid #cbd5e1',
            borderRadius: 8, fontSize: 14, marginBottom: 16, boxSizing: 'border-box',
            outline: 'none',
          }}
          autoFocus
        />
        {error && (
          <p style={{
            color: '#ef4444', fontSize: 13, marginBottom: 12,
            background: '#fef2f2', padding: '8px 12px', borderRadius: 6,
          }}>{error}</p>
        )}
        <button
          onClick={submit}
          disabled={loading}
          style={{
            width: '100%', padding: '11px', background: '#0f172a', color: '#fff',
            border: 'none', borderRadius: 8, fontSize: 14, cursor: 'pointer', fontWeight: 600,
            opacity: loading ? 0.7 : 1, transition: 'opacity 0.2s',
          }}
        >
          {loading ? '验证中...' : '登录'}
        </button>
        <p style={{ textAlign: 'center', color: '#94a3b8', fontSize: 11, marginTop: 20 }}>
          首次登录的密码将被自动设置为管理员密码
        </p>
      </div>
    </div>
  );
}
