import { useEffect, useState } from 'react';
import { api } from '../api';
import type { Node, PolicyDetail, ConfigPreview } from '../types';
import { Eye, Plus, Trash2, Edit3, AlertTriangle } from 'lucide-react';
import React from 'react';

export default function Policies() {
  const [policies, setPolicies] = useState<PolicyDetail[]>([]);
  const [nodes, setNodes] = useState<Node[]>([]);
  const [links, setLinks] = useState<any[]>([]);
  const [showCreate, setShowCreate] = useState(false);
  const [editId, setEditId] = useState<string | null>(null);
  const [preview, setPreview] = useState<ConfigPreview | null>(null);
  const [publishResult, setPublishResult] = useState<any>(null);
  const [form, setForm] = useState({
    name: '', priority: 10, description: '', enabled: true,
    source_cidr: '', dest_cidr: '', protocol: 'Any', dest_port: '',
    egress_nat: false, return_path: 'Symmetric',
  });
  const [pathNodes, setPathNodes] = useState<string[]>([]);
  const [validationErrors, setValidationErrors] = useState<string[]>([]);

  const load = async () => {
    const [p, n, l] = await Promise.all([
      api.getPolicies().catch(() => []),
      api.getNodes().catch(() => []),
      api.getLinks().catch(() => []),
    ]);
    setPolicies(p); setNodes(n); setLinks(l);
  };

  useEffect(() => { load(); }, []);

  const validatePath = (path: string[]) => {
    const errors: string[] = [];
    for (let i = 0; i < path.length - 1; i++) {
      const a = path[i], b = path[i + 1];
      const hasLink = links.some(l =>
        (l.node_a === a && l.node_b === b) || (l.node_a === b && l.node_b === a)
      );
      if (!hasLink) {
        const na = nodes.find(n => n.id === a)?.display_name || a.slice(0, 8);
        const nb = nodes.find(n => n.id === b)?.display_name || b.slice(0, 8);
        errors.push(`节点 "${na}" 与 "${nb}" 之间没有直接 WireGuard 链路`);
      }
    }
    setValidationErrors(errors);
    return errors.length === 0;
  };

  const resetForm = () => {
    setForm({ name: '', priority: 10, description: '', enabled: true, source_cidr: '', dest_cidr: '', protocol: 'Any', dest_port: '', egress_nat: false, return_path: 'Symmetric' });
    setPathNodes([]);
    setValidationErrors([]);
  };

  const create = async () => {
    if (!validatePath(pathNodes)) return;
    try {
      await api.createPolicy({
        name: form.name, priority: form.priority, description: form.description,
        matches: [{ source_cidr: form.source_cidr || null, destination_cidr: form.dest_cidr || null, protocol: form.protocol, destination_port_start: form.dest_port ? Number(form.dest_port) : null }],
        path_node_ids: pathNodes, egress_nat: form.egress_nat, return_path_type: form.return_path,
      });
      await load();
      setShowCreate(false); resetForm(); alert('策略创建成功');
    } catch (e: any) { alert('创建失败: ' + e.message); }
  };

  const update = async (id: string) => {
    if (!validatePath(pathNodes)) return;
    try {
      await api.updatePolicy(id, {
        name: form.name, priority: form.priority, description: form.description, enabled: form.enabled,
        matches: [{ source_cidr: form.source_cidr || null, destination_cidr: form.dest_cidr || null, protocol: form.protocol, destination_port_start: form.dest_port ? Number(form.dest_port) : null }],
        path_node_ids: pathNodes, egress_nat: form.egress_nat, return_path_type: form.return_path,
      });
      await load();
      setEditId(null); resetForm(); alert('策略已更新');
    } catch (e: any) { alert('更新失败: ' + e.message); }
  };

  const remove = async (id: string) => {
    if (!confirm('确认删除此策略？')) return;
    try { await api.deletePolicy(id); load(); } catch (e: any) { alert('删除失败: ' + e.message); }
  };

  const editPolicy = (p: PolicyDetail) => {
    const m = p.matches[0] || {};
    setForm({
      name: p.policy.name, priority: p.policy.priority, description: p.policy.description || '', enabled: p.policy.enabled,
      source_cidr: m.source_cidr || '', dest_cidr: m.destination_cidr || '', protocol: m.protocol || 'Any',
      dest_port: m.destination_port_start?.toString() || '', egress_nat: p.config?.egress_nat || false,
      return_path: p.config?.return_path_type || 'Symmetric',
    });
    setPathNodes(p.path.map(h => h.node_id));
    setEditId(p.policy.id);
  };

  const showPreview = async () => {
    try { const p = await api.previewConfig(); setPreview(p); setPublishResult(null); }
    catch (e: any) { alert('预览失败: ' + e.message); }
  };

  const doPublish = async () => {
    try { const r = await api.publishConfig(); setPublishResult(r); setPreview(null); alert(`配置已发布 (Revision #${r.generation})`); }
    catch (e: any) { alert('发布失败: ' + e.message); }
  };

  const isFormOpen = showCreate || editId;

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
        <h2 style={{ fontSize: 22, fontWeight: 600 }}>路径策略</h2>
        <div style={{ display: 'flex', gap: 8 }}>
          <button onClick={showPreview} style={secBtn}><Eye size={14} /> 配置预览</button>
          <button onClick={() => { resetForm(); setEditId(null); setShowCreate(!showCreate); }} style={priBtn}>
            <Plus size={14} /> 新建策略
          </button>
        </div>
      </div>

      {isFormOpen && (
        <div style={{ background: '#fff', borderRadius: 10, padding: 24, marginBottom: 16, boxShadow: '0 1px 3px rgba(0,0,0,0.06)' }}>
          <h3 style={{ fontSize: 15, fontWeight: 600, marginBottom: 16 }}>{editId ? '编辑策略' : '创建策略'}</h3>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
            <div>
              <label style={lbl}>策略名称 *</label>
              <input value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} style={inp} placeholder="例如: 网页流量" />
            </div>
            <div>
              <label style={lbl}>优先级</label>
              <input type="number" value={form.priority} onChange={e => setForm({ ...form, priority: Number(e.target.value) })} style={inp} />
            </div>
            <div>
              <label style={lbl}>源 CIDR</label>
              <input value={form.source_cidr} onChange={e => setForm({ ...form, source_cidr: e.target.value })} style={inp} placeholder="10.0.0.0/24 或留空" />
            </div>
            <div>
              <label style={lbl}>目标 CIDR</label>
              <input value={form.dest_cidr} onChange={e => setForm({ ...form, dest_cidr: e.target.value })} style={inp} placeholder="0.0.0.0/0 或留空" />
            </div>
            <div>
              <label style={lbl}>协议</label>
              <select value={form.protocol} onChange={e => setForm({ ...form, protocol: e.target.value })} style={inp}>
                <option>Any</option><option>TCP</option><option>UDP</option><option>ICMP</option>
              </select>
            </div>
            <div>
              <label style={lbl}>目标端口</label>
              <input value={form.dest_port} onChange={e => setForm({ ...form, dest_port: e.target.value })} style={inp} placeholder="443 或留空" />
            </div>
          </div>

          {editId && (
            <label style={{ ...lbl, marginTop: 10, display: 'flex', alignItems: 'center', gap: 6 }}>
              <input type="checkbox" checked={form.enabled} onChange={e => setForm({ ...form, enabled: e.target.checked })} /> 启用
            </label>
          )}

          <label style={{ ...lbl, marginTop: 14 }}>路径节点 (按顺序选择，至少2个)</label>
          <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginTop: 4 }}>
            {nodes.map(n => {
              const idx = pathNodes.indexOf(n.id);
              const selected = idx >= 0;
              return (
                <button key={n.id}
                  onClick={() => setPathNodes(selected ? pathNodes.filter(id => id !== n.id) : [...pathNodes, n.id])}
                  style={{
                    padding: '6px 12px', borderRadius: 6, border: selected ? '2px solid #3b82f6' : '1px solid #cbd5e1',
                    background: selected ? '#eff6ff' : '#fff', cursor: 'pointer', fontSize: 12,
                    color: selected ? '#1d4ed8' : '#475569',
                  }}>
                  {selected && <span style={{ marginRight: 4, fontWeight: 700 }}>{idx + 1}.</span>}
                  {n.display_name}
                </button>
              );
            })}
          </div>
          {pathNodes.length >= 2 && (
            <p style={{ fontSize: 11, color: '#64748b', marginTop: 6 }}>
              路径: {pathNodes.map(id => nodes.find(n => n.id === id)?.display_name || id.slice(0,8)).join(' → ')}
            </p>
          )}

          <div style={{ display: 'flex', gap: 16, marginTop: 14, alignItems: 'center' }}>
            <label style={{ fontSize: 13, display: 'flex', alignItems: 'center', gap: 6 }}>
              <input type="checkbox" checked={form.egress_nat} onChange={e => setForm({ ...form, egress_nat: e.target.checked })} /> 出口 NAT
            </label>
            <label style={lbl}>返回路径</label>
            <select value={form.return_path} onChange={e => setForm({ ...form, return_path: e.target.value })} style={{ ...inp, width: 140 }}>
              <option>Symmetric</option><option>Independent</option>
            </select>
          </div>

          {validationErrors.length > 0 && (
            <div style={{ background: '#fef2f2', padding: 12, borderRadius: 6, marginTop: 12, display: 'flex', flexDirection: 'column', gap: 4 }}>
              {validationErrors.map((e, i) => (
                <div key={i} style={{ fontSize: 12, color: '#dc2626', display: 'flex', alignItems: 'center', gap: 6 }}>
                  <AlertTriangle size={14} /> {e}
                </div>
              ))}
            </div>
          )}

          <button onClick={editId ? () => update(editId) : create}
            disabled={!form.name || pathNodes.length < 2}
            style={{
              ...priBtn, marginTop: 14, width: '100%', justifyContent: 'center',
              opacity: (!form.name || pathNodes.length < 2) ? 0.4 : 1,
            }}>
            {editId ? '保存修改' : '创建策略'}
          </button>
          <button onClick={() => { setShowCreate(false); setEditId(null); resetForm(); }} style={{ ...secBtn, marginTop: 8, width: '100%', justifyContent: 'center' }}>
            取消
          </button>
        </div>
      )}

      {preview && (
        <div style={{ background: '#fff', borderRadius: 10, padding: 24, marginBottom: 16, boxShadow: '0 1px 3px rgba(0,0,0,0.06)' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 12 }}>
            <div>
              <h3 style={{ fontSize: 15, fontWeight: 600 }}>配置预览 (代次 #{preview.generation})</h3>
              <span style={{ fontSize: 12, color: preview.ready ? '#16a34a' : '#f59e0b' }}>{preview.ready ? '就绪, 可以发布' : '存在验证错误'}</span>
            </div>
            <div style={{ display: 'flex', gap: 8 }}>
              <button onClick={doPublish} disabled={!preview.ready} style={{ ...priBtn, opacity: preview.ready ? 1 : 0.4 }}>发布配置</button>
              <button onClick={() => setPreview(null)} style={secBtn}>关闭</button>
            </div>
          </div>
          {preview.validation_errors.length > 0 && (
            <div style={{ background: '#fef2f2', padding: 12, borderRadius: 6, marginBottom: 12 }}>
              {preview.validation_errors.map((e, i) => (
                <p key={i} style={{ fontSize: 12, color: '#dc2626', margin: '2px 0' }}><AlertTriangle size={12} style={{ display: 'inline', marginRight: 4 }} />{e}</p>
              ))}
            </div>
          )}
          {preview.node_configs.map(nc => (
            <details key={nc.node_id} style={{ marginBottom: 8 }}>
              <summary style={{ fontSize: 13, fontWeight: 500, cursor: 'pointer', padding: '6px 0' }}>
                {nc.display_name} ({nc.overlay_ipv4}) - {nc.wireguard_links.length} 链路, {nc.policy_routing_rules.length} 路由规则
              </summary>
              <pre style={{ background: '#f8fafc', padding: 12, borderRadius: 6, fontSize: 11, overflow: 'auto', maxHeight: 200 }}>
                {JSON.stringify({ wireguard_links: nc.wireguard_links, routes: nc.route_tables, rules: nc.policy_routing_rules }, null, 2)}
              </pre>
            </details>
          ))}
        </div>
      )}

      {publishResult && (
        <div style={{ background: '#dcfce7', borderRadius: 8, padding: 12, marginBottom: 16, display: 'flex', gap: 8, alignItems: 'center' }}>
          <span style={{ fontSize: 13, color: '#16a34a', fontWeight: 600 }}>配置已发布</span>
          <span style={{ fontSize: 12, color: '#166534' }}>Revision #{publishResult.generation}, {publishResult.node_count} 个节点</span>
          <button onClick={() => setPublishResult(null)} style={{ marginLeft: 'auto', ...secBtn, fontSize: 11, padding: '3px 8px' }}>关闭</button>
        </div>
      )}

      <div style={{ background: '#fff', borderRadius: 10, overflow: 'hidden', boxShadow: '0 1px 3px rgba(0,0,0,0.06)' }}>
        <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
          <thead>
            <tr style={{ background: '#f8fafc', borderBottom: '1px solid #e2e8f0' }}>
              <th style={th}>名称</th><th style={th}>优先级</th><th style={th}>源地址</th>
              <th style={th}>目标地址</th><th style={th}>协议:端口</th><th style={th}>路径</th>
              <th style={th}>NAT</th><th style={th}>状态</th><th style={th}>操作</th>
            </tr>
          </thead>
          <tbody>
            {policies.map(p => {
              const match = p.matches[0];
              const pathStr = p.path.map(h => h.node_name || h.node_id.slice(0, 8)).join(' → ');
              return (
                <tr key={p.policy.id} style={{ borderBottom: '1px solid #f1f5f9' }}>
                  <td style={td}>{p.policy.name}</td>
                  <td style={td}>{p.policy.priority}</td>
                  <td style={{ ...td, fontFamily: 'monospace', fontSize: 11 }}>{match?.source_cidr || '*'}</td>
                  <td style={{ ...td, fontFamily: 'monospace', fontSize: 11 }}>{match?.destination_cidr || '*'}</td>
                  <td style={{ ...td, fontSize: 11 }}>{match?.protocol || 'Any'}{match?.destination_port_start ? `:${match.destination_port_start}` : ''}</td>
                  <td style={{ ...td, fontSize: 11, color: '#64748b', maxWidth: 200, overflow: 'hidden', textOverflow: 'ellipsis' }}>{pathStr}</td>
                  <td style={td}>{p.config?.egress_nat ? '是' : '否'}</td>
                  <td style={td}>
                    <span style={{ ...badge, background: p.policy.enabled ? '#dcfce7' : '#f1f5f9', color: p.policy.enabled ? '#16a34a' : '#94a3b8' }}>
                      {p.policy.enabled ? '启用' : '禁用'}
                    </span>
                  </td>
                  <td style={td}>
                    <button onClick={() => editPolicy(p)} style={iconBtn} title="编辑"><Edit3 size={14} /></button>
                    <button onClick={() => remove(p.policy.id)} style={{ ...iconBtn, color: '#dc2626' }} title="删除"><Trash2 size={14} /></button>
                  </td>
                </tr>
              );
            })}
            {policies.length === 0 && (
              <tr><td colSpan={9} style={{ padding: 24, textAlign: 'center', color: '#94a3b8' }}>暂无路径策略</td></tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}

const th: React.CSSProperties = { padding: '10px 12px', textAlign: 'left', color: '#64748b', fontWeight: 600, fontSize: 11, textTransform: 'uppercase' };
const td: React.CSSProperties = { padding: '10px 12px' };
const lbl: React.CSSProperties = { display: 'block', fontSize: 12, color: '#475569', marginBottom: 2, fontWeight: 500 };
const inp: React.CSSProperties = { width: '100%', padding: '7px 10px', border: '1px solid #cbd5e1', borderRadius: 6, fontSize: 13, boxSizing: 'border-box' };
const badge: React.CSSProperties = { padding: '2px 8px', borderRadius: 10, fontSize: 11, fontWeight: 500 };
const priBtn: React.CSSProperties = { padding: '8px 14px', background: '#0f172a', color: '#fff', border: 'none', borderRadius: 6, cursor: 'pointer', fontSize: 13, display: 'flex', alignItems: 'center', gap: 6 };
const secBtn: React.CSSProperties = { padding: '8px 14px', background: '#fff', color: '#0f172a', border: '1px solid #cbd5e1', borderRadius: 6, cursor: 'pointer', fontSize: 13, display: 'flex', alignItems: 'center', gap: 6 };
const iconBtn: React.CSSProperties = { padding: 4, background: 'transparent', border: 'none', cursor: 'pointer', color: '#64748b', marginRight: 2 };
