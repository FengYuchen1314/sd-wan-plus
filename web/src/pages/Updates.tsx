import { useEffect, useState } from 'react';
import { api } from '../api';
import type { UpdateJob } from '../types';
import { Play, RotateCcw, ChevronDown, ChevronRight } from 'lucide-react';
import React from 'react';

const statusMap: Record<string, { label: string; color: string; bg: string }> = {
  Created: { label: '已创建', color: '#2563eb', bg: '#eff6ff' },
  Downloading: { label: '下载中', color: '#7c3aed', bg: '#f5f3ff' },
  DistributingArtifacts: { label: '分发中', color: '#7c3aed', bg: '#f5f3ff' },
  Installing: { label: '安装中', color: '#ea580c', bg: '#fff7ed' },
  Completed: { label: '已完成', color: '#16a34a', bg: '#dcfce7' },
  Failed: { label: '失败', color: '#dc2626', bg: '#fef2f2' },
  RollingBack: { label: '回滚中', color: '#ea580c', bg: '#fff7ed' },
  RolledBack: { label: '已回滚', color: '#ca8a04', bg: '#fef9c3' },
  Waiting: { label: '等待中', color: '#64748b', bg: '#f1f5f9' },
};

export default function Updates() {
  const [jobs, setJobs] = useState<UpdateJob[]>([]);
  const [version, setVersion] = useState('');
  const [manifest, setManifest] = useState('');
  const [expandedJob, setExpandedJob] = useState<string | null>(null);

  const load = () => api.getUpdates().then(setJobs).catch(() => {});
  useEffect(() => { load(); }, []);

  const create = async () => {
    try { await api.createUpdate({ target_version: version, manifest_sha256: manifest }); setVersion(''); setManifest(''); load(); alert('更新作业已创建'); }
    catch (e: any) { alert('创建失败: ' + e.message); }
  };
  const start = async (id: string) => {
    try { await api.startUpdate(id); load(); alert('已启动更新 (叶子节点优先)'); }
    catch (e: any) { alert('启动失败: ' + e.message); }
  };
  const rollback = async (id: string) => {
    try { await api.rollbackUpdate(id); load(); alert('已发起回滚'); }
    catch (e: any) { alert('回滚失败: ' + e.message); }
  };

  const targetStatus = (s: string) => statusMap[s] || { label: s, color: '#64748b', bg: '#f1f5f9' };

  return (
    <div>
      <h2 style={{ fontSize: 22, fontWeight: 600, marginBottom: 16 }}>更新中心</h2>

      <div style={{ background: '#fff', borderRadius: 10, padding: 20, marginBottom: 16, boxShadow: '0 1px 3px rgba(0,0,0,0.06)', display: 'flex', gap: 12, alignItems: 'end', flexWrap: 'wrap' }}>
        <div>
          <label style={{ display: 'block', fontSize: 12, color: '#475569', marginBottom: 4, fontWeight: 500 }}>目标版本</label>
          <input value={version} onChange={e => setVersion(e.target.value)} placeholder="1.2.0" style={inp} />
        </div>
        <div>
          <label style={{ display: 'block', fontSize: 12, color: '#475569', marginBottom: 4, fontWeight: 500 }}>Manifest SHA-256</label>
          <input value={manifest} onChange={e => setManifest(e.target.value)} placeholder="abc123..." style={{ ...inp, width: 280 }} />
        </div>
        <button onClick={create} disabled={!version || !manifest} style={{
          padding: '8px 16px', background: '#0f172a', color: '#fff', border: 'none',
          borderRadius: 6, cursor: 'pointer', fontSize: 13, fontWeight: 500,
          opacity: (!version || !manifest) ? 0.4 : 1,
        }}>创建更新作业</button>
      </div>

      {jobs.map(j => (
        <div key={j.id} style={{ background: '#fff', borderRadius: 10, marginBottom: 12, boxShadow: '0 1px 3px rgba(0,0,0,0.06)', overflow: 'hidden' }}>
          <div style={{ display: 'flex', alignItems: 'center', padding: '14px 20px', cursor: 'pointer', gap: 12 }}
            onClick={() => setExpandedJob(expandedJob === j.id ? null : j.id)}>
            {React.createElement(expandedJob === j.id ? ChevronDown : ChevronRight, { size: 16, color: '#64748b' })}
            <span style={{ fontWeight: 600, fontSize: 14 }}>v{j.target_version}</span>
            <span style={{ ...badge, background: targetStatus(j.status).bg, color: targetStatus(j.status).color }}>
              {targetStatus(j.status).label}
            </span>
            <span style={{ fontSize: 12, color: '#94a3b8' }}>{new Date(j.created_at).toLocaleString()}</span>
            <div style={{ marginLeft: 'auto', display: 'flex', gap: 8 }}>
              <span style={{ fontSize: 11, color: '#64748b' }}>
                {j.summary.completed}/{j.summary.total} 完成
                {j.summary.failed > 0 && <span style={{ color: '#dc2626' }}> ({j.summary.failed} 失败)</span>}
              </span>
            </div>
          </div>

          {expandedJob === j.id && (
            <div style={{ padding: '0 20px 16px', borderTop: '1px solid #f1f5f9' }}>
              <div style={{ display: 'flex', gap: 8, padding: '10px 0' }}>
                {j.status === 'Created' && (
                  <button onClick={() => start(j.id)} style={actBtn}><Play size={12} /> 启动更新</button>
                )}
                {['DistributingArtifacts', 'Installing', 'Failed'].includes(j.status) && (
                  <button onClick={() => rollback(j.id)} style={{ ...actBtn, background: '#fef2f2', color: '#dc2626' }}>
                    <RotateCcw size={12} /> 回滚</button>
                )}
              </div>
              <div style={{ marginTop: 8 }}>
                <div style={{ fontSize: 11, color: '#94a3b8', marginBottom: 8 }}>更新目标 ({j.targets.length} 个节点，深度排序，叶子优先)</div>
                {j.targets.sort((a, b) => b.depth - a.depth).map(t => {
                  const ts = targetStatus(t.status);
                  return (
                    <div key={t.node_id} style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '6px 0', borderBottom: '1px solid #f8fafc', fontSize: 13 }}>
                      <span>{t.node_name || t.node_id.slice(0,8)}</span>
                      <span style={{ fontSize: 10, color: '#94a3b8' }}>深度 {t.depth}</span>
                      <span style={{ ...badge, background: ts.bg, color: ts.color, fontSize: 10 }}>{ts.label}</span>
                      {t.error_message && <span style={{ fontSize: 11, color: '#dc2626' }}>{t.error_message}</span>}
                    </div>
                  );
                })}
              </div>
            </div>
          )}
        </div>
      ))}
      {jobs.length === 0 && (
        <div style={{ padding: 40, textAlign: 'center', color: '#94a3b8', background: '#fff', borderRadius: 10, boxShadow: '0 1px 3px rgba(0,0,0,0.06)' }}>
          暂无更新作业
        </div>
      )}
    </div>
  );
}

const inp: React.CSSProperties = { padding: '7px 10px', border: '1px solid #cbd5e1', borderRadius: 6, fontSize: 13, width: 140, boxSizing: 'border-box' };
const badge: React.CSSProperties = { padding: '3px 10px', borderRadius: 10, fontSize: 11, fontWeight: 500 };
const actBtn: React.CSSProperties = { padding: '5px 12px', background: '#eff6ff', color: '#2563eb', border: 'none', borderRadius: 4, cursor: 'pointer', fontSize: 12, display: 'inline-flex', alignItems: 'center', gap: 4, fontWeight: 500 };
