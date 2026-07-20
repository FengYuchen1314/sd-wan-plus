import { useEffect, useState } from 'react';
import { api } from '../api';
import type { Node } from '../types';

export default function NodesPage() {
  const [nodes, setNodes] = useState<Node[]>([]);
  const [renameId, setRenameId] = useState<string | null>(null);
  const [newName, setNewName] = useState('');

  const load = () => api.getNodes().then(setNodes).catch(() => {});
  useEffect(() => { load(); }, []);

  const rename = async (id: string) => {
    try { await api.renameNode(id, newName); load(); setRenameId(null); }
    catch (e) { alert('重命名失败: ' + e); }
  };

  return (
    <div>
      <h2 style={{ fontSize: 22, fontWeight: 600, marginBottom: 16 }}>节点管理</h2>
      <div style={{ background: '#fff', borderRadius: 10, overflow: 'hidden', boxShadow: '0 1px 3px rgba(0,0,0,0.08)' }}>
        <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
          <thead>
            <tr style={{ background: '#f8fafc', borderBottom: '1px solid #e2e8f0' }}>
              <th style={th}>名称</th><th style={th}>Overlay IP</th><th style={th}>角色</th>
              <th style={th}>服务端口</th><th style={th}>WG 端口范围</th>
              <th style={th}>状态</th><th style={th}>操作</th>
            </tr>
          </thead>
          <tbody>
            {nodes.map(n => (
              <tr key={n.id} style={{ borderBottom: '1px solid #f1f5f9' }}>
                <td style={td}>
                  {renameId === n.id ? (
                    <input value={newName} onChange={e => setNewName(e.target.value)}
                      style={{ width: 100, padding: '4px 6px', border: '1px solid #cbd5e1', borderRadius: 4, fontSize: 12 }} />
                  ) : n.display_name}
                </td>
                <td style={{ ...td, fontFamily: 'monospace', fontSize: 12 }}>{n.overlay_ipv4}</td>
                <td style={td}>
                  <span style={{
                    padding: '2px 8px', borderRadius: 10, fontSize: 11,
                    background: n.is_controller ? '#fef3c7' : '#e0f2fe',
                    color: n.is_controller ? '#92400e' : '#0369a1',
                  }}>{n.is_controller ? '控制机' : '节点'}</span>
                </td>
                <td style={{ ...td, fontFamily: 'monospace' }}>{n.node_service_port}</td>
                <td style={{ ...td, fontFamily: 'monospace' }}>30000-30999</td>
                <td style={td}>
                  <span style={{ color: n.last_seen_at ? '#16a34a' : '#94a3b8', fontSize: 12 }}>
                    {n.last_seen_at ? '在线' : '离线'}
                  </span>
                </td>
                <td style={td}>
                  {renameId === n.id ? (
                    <>
                      <button onClick={() => rename(n.id)} style={btn}>保存</button>
                      <button onClick={() => setRenameId(null)} style={{ ...btn, background: '#f1f5f9', color: '#475569' }}>取消</button>
                    </>
                  ) : (
                    <button onClick={() => { setRenameId(n.id); setNewName(n.display_name); }} style={btn}>重命名</button>
                  )}
                </td>
              </tr>
            ))}
            {nodes.length === 0 && (
              <tr><td colSpan={7} style={{ padding: 24, textAlign: 'center', color: '#94a3b8' }}>暂无已接入节点</td></tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}

const th: React.CSSProperties = { padding: '10px 16px', textAlign: 'left', color: '#64748b', fontWeight: 600, fontSize: 12 };
const td: React.CSSProperties = { padding: '10px 16px' };
const btn: React.CSSProperties = {
  padding: '4px 10px', background: '#0f172a', color: '#fff',
  border: 'none', borderRadius: 4, cursor: 'pointer', fontSize: 12, marginRight: 4,
};
