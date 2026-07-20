import type { ReactNode } from 'react';
import { NavLink, useNavigate } from 'react-router-dom';
import { LayoutDashboard, Share2, Server, UserPlus, FileText, RefreshCw, Settings, LogOut } from 'lucide-react';
import { api } from './api';

const nav = [
  { to: '/', icon: LayoutDashboard, label: '仪表盘' },
  { to: '/topology', icon: Share2, label: '拓扑图' },
  { to: '/nodes', icon: Server, label: '节点管理' },
  { to: '/enrollment', icon: UserPlus, label: '接入新节点' },
  { to: '/policies', icon: FileText, label: '路径策略' },
  { to: '/updates', icon: RefreshCw, label: '更新中心' },
];

export default function Layout({ children }: { children: ReactNode }) {
  const navigate = useNavigate();

  const logout = async () => {
    try { await api.logout(); } catch { /* ignore */ }
    navigate('/login', { replace: true });
  };

  return (
    <div style={{ display: 'flex', height: '100vh' }}>
      <aside style={{
        width: 220, background: '#0f172a', color: '#e2e8f0',
        display: 'flex', flexDirection: 'column', padding: '16px 0',
      }}>
        <div style={{ padding: '0 16px 24px', fontSize: 18, fontWeight: 700, color: '#38bdf8' }}>
          PathWeaver
        </div>
        {nav.map(({ to, icon: Icon, label }) => (
          <NavLink
            key={to}
            to={to}
            style={({ isActive }) => ({
              display: 'flex', alignItems: 'center', gap: 12,
              padding: '10px 20px', color: isActive ? '#38bdf8' : '#94a3b8',
              background: isActive ? 'rgba(56,189,248,0.1)' : 'transparent',
              textDecoration: 'none', fontSize: 14,
              borderLeft: isActive ? '3px solid #38bdf8' : '3px solid transparent',
            })}
          >
            <Icon size={18} /> {label}
          </NavLink>
        ))}
        <div style={{ flex: 1 }} />
        <NavLink
          to="/settings"
          style={({ isActive }) => ({
            display: 'flex', alignItems: 'center', gap: 12,
            padding: '10px 20px', color: isActive ? '#38bdf8' : '#94a3b8',
            background: isActive ? 'rgba(56,189,248,0.1)' : 'transparent',
            textDecoration: 'none', fontSize: 13,
            borderLeft: isActive ? '3px solid #38bdf8' : '3px solid transparent',
          })}
        >
          <Settings size={18} /> 系统设置
        </NavLink>
        <button onClick={logout} style={{
          display: 'flex', alignItems: 'center', gap: 12, margin: '8px 16px 0',
          padding: '8px 12px', background: 'transparent', border: '1px solid #334155',
          borderRadius: 6, color: '#94a3b8', cursor: 'pointer', fontSize: 13,
        }}>
          <LogOut size={16} /> 退出登录
        </button>
      </aside>
      <main style={{ flex: 1, overflow: 'auto', background: '#f1f5f9', padding: 24 }}>
        {children}
      </main>
    </div>
  );
}
