import { useEffect, useState } from 'react';
import { api } from '../api';
import type { Node, PolicyDetail, ConfigPreview } from '../types';
import { Eye, Plus } from 'lucide-react';

export default function Policies() {
  const [policies, setPolicies] = useState<PolicyDetail[]>([]);
  const [nodes, setNodes] = useState<Node[]>([]);
  const [showCreate, setShowCreate] = useState(false);
  const [preview, setPreview] = useState<ConfigPreview | null>(null);
  const [form, setForm] = useState({
    name: '', priority: 10, description: '',
    source_cidr: '', dest_cidr: '', protocol: 'Any', dest_port: '',
    egress_nat: false, return_path: 'Symmetric',
  });
  const [pathNodes, setPathNodes] = useState<string[]>([]);

  useEffect(() => {
    api.getPolicies().then(setPolicies).catch(() => {});
    api.getNodes().then(setNodes).catch(() => {});
  }, []);

  const create = async () => {
    try {
      await api.createPolicy({
        name: form.name,
        priority: form.priority,
        description: form.description,
        matches: [{
          source_cidr: form.source_cidr || null,
          destination_cidr: form.dest_cidr || null,
          protocol: form.protocol,
          destination_port: form.dest_port ? Number(form.dest_port) : null,
        }],
        path_node_ids: pathNodes,
        egress_nat: form.egress_nat,
        return_path_type: form.return_path,
      });
      const p = await api.getPolicies();
      setPolicies(p); setShowCreate(false); setPathNodes([]);
      alert('策略创建成功');
    } catch (e) { alert('创建失败: ' + e); }
  };

  const showPreview = async () => {
    try { setPreview(await api.previewConfig()); }
    catch (e) { alert('预览失败: ' + e); }
  };

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
        <h2 style={{ fontSize: 22, fontWeight: 600 }}>路径策略</h2>
        <div style={{ display: 'flex', gap: 8 }}>
          <button onClick={showPreview} style={secBtn}><Eye size={14} /> 配置预览</button>
          <button onClick={() => setShowCreate(!showCreate)} style={priBtn}><Plus size={14} /> 新建策略</button>
        </div>
      </div>

      {showCreate && (
        <div style={{ background: '#fff', borderRadius: 10, padding: 24, marginBottom: 16, boxShadow: '0 1px 3px rgba(0,0,0,0.08)' }}>
          <h3 style={{ fontSize: 15, fontWeight: 600, marginBottom: 16 }}>创建策略</h3>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
            <div><label style={lbl}>策略名称</label>
              <input value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} placeholder="例如: 网页流量" style={inp} /></div>
            <div><label style={lbl}>优先级</label>
              <input type="number" value={form.priority} onChange={e => setForm({ ...form, priority: Number(e.target.value) })} style={inp} /></div>
            <div><label style={lbl}>源 CIDR</label>
              <input value={form.source_cidr} onChange={e => setForm({ ...form, source_cidr: e.target.value })} placeholder="10.0.0.0/24" style={inp} /></div>
            <div><label style={lbl}>目标 CIDR</label>
              <input value={form.dest_cidr} onChange={e => setForm({ ...form, dest_cidr: e.target.value })} placeholder="0.0.0.0/0" style={inp} /></div>
            <div><label style={lbl}>协议</label>
              <select value={form.protocol} onChange={e => setForm({ ...form, protocol: e.target.value })} style={inp}>
                <option>Any</option><option>TCP</option><option>UDP</option><option>ICMP</option></select></div>
            <div><label style={lbl}>目标端口</label>
              <input value={form.dest_port} onChange={e => setForm({ ...form, dest_port: e.target.value })} placeholder="443" style={inp} /></div>
          </div>

          <label style={{ ...lbl, marginTop: 14 }}>路径节点 (按顺序选择)</label>
          <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginTop: 4 }}>
            {nodes.map(n => {
              const selected = pathNodes.includes(n.id);
              return (
                <button key={n.id}
                  onClick={() => setPathNodes(selected ? pathNodes.filter(id => id !== n.id) : [...pathNodes, n.id])}
                  style={{
                    padding: '6px 12px', borderRadius: 6, border: selected ? '2px solid #3b82f6' : '1px solid #cbd5e1',
                    background: selected ? '#eff6ff' : '#fff', cursor: 'pointer', fontSize: 12,
                    color: selected ? '#1d4ed8' : '#475569',
                  }}>
                  {pathNodes.indexOf(n.id) >= 0 && <span style={{ marginRight: 4, fontWeight: 700 }}>{pathNodes.indexOf(n.id) + 1}.</span>}
                  {n.display_name}
                </button>
              );
            })}
          </div>

          <div style={{ display: 'flex', gap: 16, marginTop: 14, alignItems: 'center' }}>
            <label style={{ fontSize: 13, display: 'flex', alignItems: 'center', gap: 6 }}>
              <input type="checkbox" checked={form.egress_nat} onChange={e => setForm({ ...form, egress_nat: e.target.checked })} /> 出口 NAT
            </label>
            <label style={{ fontSize: 13, color: '#475569' }}>返回路径</label>
            <select value={form.return_path} onChange={e => setForm({ ...form, return_path: e.target.value })} style={{ ...inp, width: 140 }}>
              <option>Symmetric</option><option>Independent</option></select>
          </div>

          <button onClick={create} disabled={!form.name || pathNodes.length < 2} style={{
            ...priBtn, marginTop: 16, width: '100%',
            opacity: (!form.name || pathNodes.length < 2) ? 0.4 : 1,
          }}>创建策略</button>
        </div>
      )}

      {preview && (
        <div style={{ background: '#fff', borderRadius: 10, padding: 24, marginBottom: 16, boxShadow: '0 1px 3px rgba(0,0,0,0.08)' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 12 }}>
            <h3 style={{ fontSize: 15, fontWeight: 600 }}>配置预览 (代次 #{preview.generation})</h3>
            <button onClick={() => setPreview(null)} style={{ ...secBtn, fontSize: 12 }}>关闭</button>
          </div>
          {preview.validation_errors.length > 0 && (
            <div style={{ background: '#fef2f2', padding: 12, borderRadius: 6, marginBottom: 12 }}>
              {preview.validation_errors.map((e, i) => (
                <p key={i} style={{ fontSize: 12, color: '#dc2626', margin: '2px 0' }}>{e}</p>
              ))}
            </div>
          )}
          {preview.node_configs.map(nc => (
            <details key={nc.node_id} style={{ marginBottom: 8 }}>
              <summary style={{ fontSize: 13, fontWeight: 500, cursor: 'pointer', padding: '6px 0' }}>
                {nc.display_name} ({nc.overlay_ip}) - {nc.wireguard_links.length} 链路, {nc.policy_routing_rules.length} 路由规则
              </summary>
              <pre style={{ background: '#f8fafc', padding: 12, borderRadius: 6, fontSize: 11, overflow: 'auto', maxHeight: 200 }}>
                {JSON.stringify({ wireguard_links: nc.wireguard_links, routes: nc.route_tables, rules: nc.policy_routing_rules }, null, 2)}
              </pre>
            </details>
          ))}
        </div>
      )}

      <div style={{ background: '#fff', borderRadius: 10, overflow: 'hidden', boxShadow: '0 1px 3px rgba(0,0,0,0.08)' }}>
        <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
          <thead>
            <tr style={{ background: '#f8fafc', borderBottom: '1px solid #e2e8f0' }}>
              <th style={th}>名称</th><th style={th}>优先级</th><th style={th}>源地址</th>
              <th style={th}>目标地址</th><th style={th}>协议</th><th style={th}>路径</th><th style={th}>NAT</th>
            </tr>
          </thead>
          <tbody>
            {policies.map(p => {
              const match = p.matches[0];
              const pathStr = p.path.map(h => nodes.find(n => n.id === h.node_id)?.display_name || h.node_id.slice(0, 8)).join(' → ');
              return (
                <tr key={p.policy.id} style={{ borderBottom: '1px solid #f1f5f9' }}>
                  <td style={td}>{p.policy.name}</td><td style={td}>{p.policy.priority}</td>
                  <td style={{ ...td, fontFamily: 'monospace', fontSize: 11 }}>{match?.source_cidr || '*'}</td>
                  <td style={{ ...td, fontFamily: 'monospace', fontSize: 11 }}>{match?.destination_cidr || '*'}</td>
                  <td style={td}>{match?.protocol || 'Any'}{match?.destination_port_start ? `:${match.destination_port_start}` : ''}</td>
                  <td style={{ ...td, fontSize: 11, color: '#64748b' }}>{pathStr}</td>
                  <td style={td}>{p.config?.egress_nat ? '是' : '否'}</td>
                </tr>
              );
            })}
            {policies.length === 0 && (
              <tr><td colSpan={7} style={{ padding: 24, textAlign: 'center', color: '#94a3b8' }}>暂无路径策略</td></tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}

const th: React.CSSProperties = { padding: '10px 14px', textAlign: 'left', color: '#64748b', fontWeight: 600, fontSize: 12 };
const td: React.CSSProperties = { padding: '10px 14px' };
const lbl: React.CSSProperties = { display: 'block', fontSize: 12, color: '#475569', marginBottom: 2 };
const inp: React.CSSProperties = { width: '100%', padding: '7px 10px', border: '1px solid #cbd5e1', borderRadius: 6, fontSize: 13, boxSizing: 'border-box' };
const priBtn: React.CSSProperties = {
  padding: '8px 14px', background: '#0f172a', color: '#fff', border: 'none',
  borderRadius: 6, cursor: 'pointer', fontSize: 13, display: 'flex', alignItems: 'center', gap: 6,
};
const secBtn: React.CSSProperties = {
  padding: '8px 14px', background: '#fff', color: '#0f172a', border: '1px solid #cbd5e1',
  borderRadius: 6, cursor: 'pointer', fontSize: 13, display: 'flex', alignItems: 'center', gap: 6,
};
