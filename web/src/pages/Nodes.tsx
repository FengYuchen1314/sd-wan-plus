import { useEffect, useState } from 'react';
import { api } from '../api';
import type { Node } from '../types';
import { MapPin } from 'lucide-react';

export default function NodesPage() {
  const [nodes, setNodes] = useState<Node[]>([]);
  const [renameId, setRenameId] = useState<string | null>(null);
  const [newName, setNewName] = useState('');
  const [addrNodeId, setAddrNodeId] = useState<string | null>(null);
  const [addrForm, setAddrForm] = useState({ address: '', address_type: 'public' });
  const [refreshKey, setRefreshKey] = useState(0);

  const load = () => api.getNodes().then(setNodes).catch(() => {});
  useEffect(() => { load(); }, [refreshKey]);

  const rename = async (id: string) => {
    try { await api.renameNode(id, newName); load(); setRenameId(null); }
    catch (e: any) { alert('重命名失败: ' + e.message); }
  };

  const addAddress = async () => {
    if (!addrNodeId) return;
    const node = nodes.find(n => n.id === addrNodeId);
    if (!node) return;
    const addresses = [...node.addresses.map(a => ({
      address: a.address, type: a.address_type, primary: a.is_primary,
    })), {
      address: addrForm.address, type: addrForm.address_type, primary: node.addresses.length === 0,
    }];
    try {
      await api.updateNode(addrNodeId, { addresses });
      load(); setAddrNodeId(null); setAddrForm({ address: '', address_type: 'public' });
    } catch (e: any) { alert('更新地址失败: ' + e.message); }
  };

  const removeAddress = async (nodeId: string, addrIdx: number) => {
    const node = nodes.find(n => n.id === nodeId);
    if (!node) return;
    const addresses = node.addresses.filter((_, i) => i !== addrIdx).map((a, i) => ({
      address: a.address, type: a.address_type, primary: i === 0,
    }));
    try {
      await api.updateNode(nodeId, { addresses });
      load();
    } catch (e: any) { alert('删除地址失败: ' + e.message); }
  };

  return (
    <div>
      <h2 style={{ fontSize: 22, fontWeight: 600, marginBottom: 16 }}>节点管理</h2>
      <div style={{ background: '#fff', borderRadius: 10, overflow: 'hidden', boxShadow: '0 1px 3px rgba(0,0,0,0.06)' }}>
        <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
          <thead>
            <tr style={{ background: '#f8fafc', borderBottom: '1px solid #e2e8f0' }}>
              <th style={th}>名称</th><th style={th}>Overlay IP</th><th style={th}>角色</th>
              <th style={th}>父节点</th><th style={th}>可达地址</th>
              <th style={th}>WG 端口</th><th style={th}>状态</th><th style={th}>操作</th>
            </tr>
          </thead>
          <tbody>
            {nodes.map(n => (
              <tr key={n.id} style={{ borderBottom: '1px solid #f1f5f9' }}>
                <td style={td}>
                  {renameId === n.id ? (
                    <input value={newName} onChange={e => setNewName(e.target.value)}
                      style={smallInput} autoFocus />
                  ) : n.display_name}
                </td>
                <td style={{ ...td, fontFamily: 'monospace', fontSize: 12 }}>{n.overlay_ipv4}</td>
                <td style={td}>
                  <span style={{ ...badge, background: n.is_controller ? '#fef3c7' : '#e0f2fe', color: n.is_controller ? '#92400e' : '#0369a1' }}>
                    {n.is_controller ? '控制机' : '节点'}
                  </span>
                </td>
                <td style={td}>{n.control_parent || '-'}</td>
                <td style={td}>
                  <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
                    {n.addresses?.map((a, i) => (
                      <span key={i} style={{ ...badge, background: '#f1f5f9', color: '#475569', fontSize: 11, display: 'inline-flex', alignItems: 'center', gap: 4 }}>
                        {a.address}
                        <span onClick={() => removeAddress(n.id, i)} style={{ cursor: 'pointer', color: '#94a3b8', fontWeight: 700 }}>&times;</span>
                      </span>
                    ))}
                    <button onClick={() => { setAddrNodeId(n.id); setAddrForm({ address: '', address_type: 'public' }); }}
                      style={{ ...smallBtn, padding: '1px 6px', fontSize: 11 }}>+</button>
                  </div>
                  {addrNodeId === n.id && (
                    <div style={{ display: 'flex', gap: 4, marginTop: 4 }}>
                      <input value={addrForm.address} onChange={e => setAddrForm({ ...addrForm, address: e.target.value })}
                        placeholder="IP 地址" style={{ ...smallInput, width: 110 }} />
                      <select value={addrForm.address_type} onChange={e => setAddrForm({ ...addrForm, address_type: e.target.value })}
                        style={{ ...smallInput, width: 70 }}>
                        <option value="public">公网</option><option value="private">内网</option><option value="domain">域名</option>
                      </select>
                      <button onClick={addAddress} style={smallBtn}>保存</button>
                      <button onClick={() => setAddrNodeId(null)} style={{ ...smallBtn, background: '#f1f5f9', color: '#475569' }}>取消</button>
                    </div>
                  )}
                </td>
                <td style={{ ...td, fontFamily: 'monospace', fontSize: 12 }}>{n.wg_port_range_start}-{n.wg_port_range_end}</td>
                <td style={td}>
                  <div style={{ fontSize: 11, lineHeight: 1.4 }}>
                    <div><span style={{ color: n.last_seen_at ? '#16a34a' : '#94a3b8' }}>{n.last_seen_at ? 'Agent 在线' : 'Agent 离线'}</span></div>
                    <div><span style={{ color: n.desired_generation === n.active_generation ? '#16a34a' : '#f59e0b' }}>
                      {n.desired_generation === n.active_generation ? '配置一致' : '配置待同步'}
                    </span></div>
                  </div>
                </td>
                <td style={td}>
                  {renameId === n.id ? (
                    <>
                      <button onClick={() => rename(n.id)} style={priBtn}>保存</button>
                      <button onClick={() => setRenameId(null)} style={secBtn}>取消</button>
                    </>
                  ) : (
                    <button onClick={() => { setRenameId(n.id); setNewName(n.display_name); }} style={priBtn}>重命名</button>
                  )}
                </td>
              </tr>
            ))}
            {nodes.length === 0 && (
              <tr><td colSpan={8} style={{ padding: 24, textAlign: 'center', color: '#94a3b8' }}>暂无已接入节点</td></tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}

const th: React.CSSProperties = { padding: '10px 14px', textAlign: 'left', color: '#64748b', fontWeight: 600, fontSize: 11, textTransform: 'uppercase' };
const td: React.CSSProperties = { padding: '9px 14px' };
const badge: React.CSSProperties = { padding: '2px 8px', borderRadius: 10, fontSize: 11, fontWeight: 500 };
const smallInput: React.CSSProperties = { padding: '4px 6px', border: '1px solid #cbd5e1', borderRadius: 4, fontSize: 12, boxSizing: 'border-box' };
const smallBtn: React.CSSProperties = { padding: '4px 8px', background: '#0f172a', color: '#fff', border: 'none', borderRadius: 4, cursor: 'pointer', fontSize: 11 };
const priBtn: React.CSSProperties = { padding: '4px 10px', background: '#0f172a', color: '#fff', border: 'none', borderRadius: 4, cursor: 'pointer', fontSize: 12, marginRight: 4 };
const secBtn: React.CSSProperties = { padding: '4px 10px', background: '#f1f5f9', color: '#475569', border: 'none', borderRadius: 4, cursor: 'pointer', fontSize: 12 };
