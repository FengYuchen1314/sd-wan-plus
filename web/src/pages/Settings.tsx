import { useEffect, useState } from 'react';
import { api } from '../api';
import type { SessionInfo } from '../types';
import { Key, Shield, Monitor } from 'lucide-react';
import React from 'react';

export default function Settings() {
  const [sessions, setSessions] = useState<SessionInfo[]>([]);
  const [pwForm, setPwForm] = useState({ current: '', newPass: '', confirm: '' });
  const [pwError, setPwError] = useState('');
  const [pwSuccess, setPwSuccess] = useState('');
  const [revoking, setRevoking] = useState(false);

  const loadSessions = () => api.getSessions().then(setSessions).catch(() => {});
  useEffect(() => { loadSessions(); }, []);

  const changePassword = async () => {
    setPwError(''); setPwSuccess('');
    if (pwForm.newPass.length < 8) { setPwError('新密码至少需要8个字符'); return; }
    if (pwForm.newPass !== pwForm.confirm) { setPwError('两次输入的新密码不一致'); return; }
    try {
      await api.changePassword(pwForm.current, pwForm.newPass);
      setPwSuccess('密码修改成功');
      setPwForm({ current: '', newPass: '', confirm: '' });
    } catch (e: any) { setPwError(e.message || '修改失败'); }
  };

  const revokeAll = async () => {
    if (!confirm('确认撤销所有其他会话？当前会话不受影响。')) return;
    setRevoking(true);
    try {
      await api.revokeSessions();
      loadSessions();
      alert('其他会话已全部撤销');
    } catch (e: any) { alert('撤销失败: ' + e.message); }
    finally { setRevoking(false); }
  };

  return (
    <div>
      <h2 style={{ fontSize: 22, fontWeight: 600, marginBottom: 20 }}>系统设置</h2>

      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
        <div style={{ background: '#fff', borderRadius: 10, padding: 24, boxShadow: '0 1px 3px rgba(0,0,0,0.06)' }}>
          <h3 style={{ fontSize: 15, fontWeight: 600, marginBottom: 16, display: 'flex', alignItems: 'center', gap: 8 }}>
            <Key size={18} /> 修改管理员密码
          </h3>
          <div>
            <label style={lbl}>当前密码</label>
            <input type="password" value={pwForm.current} onChange={e => setPwForm({ ...pwForm, current: e.target.value })}
              style={inp} placeholder="输入当前密码" />
          </div>
          <div style={{ marginTop: 12 }}>
            <label style={lbl}>新密码</label>
            <input type="password" value={pwForm.newPass} onChange={e => setPwForm({ ...pwForm, newPass: e.target.value })}
              style={inp} placeholder="至少8个字符" />
          </div>
          <div style={{ marginTop: 12 }}>
            <label style={lbl}>确认新密码</label>
            <input type="password" value={pwForm.confirm} onChange={e => setPwForm({ ...pwForm, confirm: e.target.value })}
              style={inp} placeholder="再次输入新密码" />
          </div>
          {pwError && <p style={{ color: '#dc2626', fontSize: 12, marginTop: 8, background: '#fef2f2', padding: '8px 12px', borderRadius: 6 }}>{pwError}</p>}
          {pwSuccess && <p style={{ color: '#16a34a', fontSize: 12, marginTop: 8, background: '#dcfce7', padding: '8px 12px', borderRadius: 6 }}>{pwSuccess}</p>}
          <button onClick={changePassword} disabled={!pwForm.current || !pwForm.newPass}
            style={{
              marginTop: 14, padding: '10px 20px', background: '#0f172a', color: '#fff', border: 'none',
              borderRadius: 6, cursor: 'pointer', fontSize: 13, fontWeight: 600,
              opacity: (!pwForm.current || !pwForm.newPass) ? 0.5 : 1,
            }}>修改密码</button>
        </div>

        <div style={{ background: '#fff', borderRadius: 10, padding: 24, boxShadow: '0 1px 3px rgba(0,0,0,0.06)' }}>
          <h3 style={{ fontSize: 15, fontWeight: 600, marginBottom: 16, display: 'flex', alignItems: 'center', gap: 8 }}>
            <Monitor size={18} /> 活动会话
          </h3>
          <button onClick={revokeAll} disabled={revoking} style={{
            marginBottom: 14, padding: '8px 16px', background: '#fef2f2', color: '#dc2626', border: '1px solid #fecaca',
            borderRadius: 6, cursor: 'pointer', fontSize: 13, fontWeight: 500,
          }}>
            <Shield size={14} style={{ display: 'inline', marginRight: 6 }} />
            {revoking ? '撤销中...' : '撤销所有其他会话'}
          </button>
          {sessions.filter(s => !s.revoked).map(s => (
            <div key={s.id} style={{ padding: '10px 0', borderBottom: '1px solid #f1f5f9', fontSize: 12 }}>
              <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                <span style={{ color: '#475569' }}>{s.ip_address || '未知 IP'}</span>
                <span style={{ color: '#94a3b8', fontSize: 11 }}>
                  {new Date(s.created_at).toLocaleString()}
                </span>
              </div>
              <div style={{ color: '#94a3b8', fontSize: 11, marginTop: 2, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                {s.user_agent || '未知客户端'}
              </div>
              <div style={{ color: '#94a3b8', fontSize: 10, marginTop: 1 }}>
                过期: {new Date(s.expires_at).toLocaleString()}
              </div>
            </div>
          ))}
          {sessions.filter(s => !s.revoked).length === 0 && (
            <p style={{ fontSize: 13, color: '#94a3b8', textAlign: 'center', padding: 16 }}>仅当前会话活跃</p>
          )}
        </div>
      </div>
    </div>
  );
}

const lbl: React.CSSProperties = { display: 'block', fontSize: 12, color: '#475569', marginBottom: 4, fontWeight: 500 };
const inp: React.CSSProperties = { width: '100%', padding: '8px 10px', border: '1px solid #cbd5e1', borderRadius: 6, fontSize: 13, boxSizing: 'border-box' };
