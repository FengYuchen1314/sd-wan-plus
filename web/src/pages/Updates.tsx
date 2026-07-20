import { useEffect, useState } from 'react';
import { api } from '../api';
import type { UpdateJob } from '../types';
import { Play, RotateCcw } from 'lucide-react';

export default function Updates() {
  const [jobs, setJobs] = useState<UpdateJob[]>([]);
  const [version, setVersion] = useState('');
  const [manifest, setManifest] = useState('');

  const load = () => api.getUpdates().then(setJobs).catch(() => {});
  useEffect(() => { load(); }, []);

  const create = async () => {
    try { await api.createUpdate({ target_version: version, manifest_sha256: manifest }); setVersion(''); setManifest(''); load(); alert('更新作业已创建'); }
    catch (e) { alert('创建失败: ' + e); }
  };
  const start = async (id: string) => {
    try { await api.startUpdate(id); load(); alert('已启动更新 (从叶子节点开始)'); }
    catch (e) { alert('启动失败: ' + e); }
  };
  const rollback = async (id: string) => {
    try { await api.rollbackUpdate(id); load(); alert('已发起回滚'); }
    catch (e) { alert('回滚失败: ' + e); }
  };

  const statusMap: Record<string, string> = {
    Created: '已创建', Downloading: '下载中', DistributingArtifacts: '分发中',
    Installing: '安装中', Completed: '已完成', Failed: '失败',
    RollingBack: '回滚中', RolledBack: '已回滚',
  };

  return (
    <div>
      <h2 style={{ fontSize: 22, fontWeight: 600, marginBottom: 16 }}>更新中心</h2>
      <div style={{ background: '#fff', borderRadius: 10, padding: 20, marginBottom: 16, boxShadow: '0 1px 3px rgba(0,0,0,0.08)', display: 'flex', gap: 12, alignItems: 'end' }}>
        <div>
          <label style={{ display: 'block', fontSize: 12, color: '#475569', marginBottom: 4 }}>目标版本</label>
          <input value={version} onChange={e => setVersion(e.target.value)} placeholder="1.2.0" style={inp} />
        </div>
        <div>
          <label style={{ display: 'block', fontSize: 12, color: '#475569', marginBottom: 4 }}>Manifest SHA-256</label>
          <input value={manifest} onChange={e => setManifest(e.target.value)} placeholder="abc123..." style={{ ...inp, width: 260 }} />
        </div>
        <button onClick={create} disabled={!version || !manifest} style={{
          padding: '8px 16px', background: '#0f172a', color: '#fff', border: 'none',
          borderRadius: 6, cursor: 'pointer', fontSize: 13,
          opacity: (!version || !manifest) ? 0.4 : 1,
        }}>创建更新作业</button>
      </div>

      <div style={{ background: '#fff', borderRadius: 10, overflow: 'hidden', boxShadow: '0 1px 3px rgba(0,0,0,0.08)' }}>
        <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
          <thead>
            <tr style={{ background: '#f8fafc', borderBottom: '1px solid #e2e8f0' }}>
              <th style={th}>目标版本</th><th style={th}>状态</th><th style={th}>创建时间</th><th style={th}>操作</th>
            </tr>
          </thead>
          <tbody>
            {jobs.map(j => (
              <tr key={j.id} style={{ borderBottom: '1px solid #f1f5f9' }}>
                <td style={td}>{j.target_version}</td>
                <td style={td}>
                  <span style={{
                    padding: '3px 10px', borderRadius: 10, fontSize: 11,
                    background: j.status === 'Completed' ? '#dcfce7' : j.status === 'Failed' ? '#fef2f2' : '#eff6ff',
                    color: j.status === 'Completed' ? '#16a34a' : j.status === 'Failed' ? '#dc2626' : '#2563eb',
                  }}>{statusMap[j.status] || j.status}</span>
                </td>
                <td style={td}>{new Date(j.created_at).toLocaleString()}</td>
                <td style={td}>
                  {j.status === 'Created' && (
                    <button onClick={() => start(j.id)} style={actBtn}><Play size={12} /> 启动</button>
                  )}
                  {['DistributingArtifacts', 'Installing', 'Failed'].includes(j.status) && (
                    <button onClick={() => rollback(j.id)} style={{ ...actBtn, background: '#fef2f2', color: '#dc2626' }}>
                      <RotateCcw size={12} /> 回滚</button>
                  )}
                </td>
              </tr>
            ))}
            {jobs.length === 0 && (
              <tr><td colSpan={4} style={{ padding: 24, textAlign: 'center', color: '#94a3b8' }}>暂无更新作业</td></tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}

const th: React.CSSProperties = { padding: '10px 16px', textAlign: 'left', color: '#64748b', fontWeight: 600, fontSize: 12 };
const td: React.CSSProperties = { padding: '10px 16px' };
const inp: React.CSSProperties = { padding: '7px 10px', border: '1px solid #cbd5e1', borderRadius: 6, fontSize: 13, width: 140 };
const actBtn: React.CSSProperties = {
  padding: '5px 10px', background: '#eff6ff', color: '#2563eb', border: 'none',
  borderRadius: 4, cursor: 'pointer', fontSize: 12, display: 'inline-flex', alignItems: 'center', gap: 4,
};
