import { useEffect, useState } from 'react';
import { api } from '../api';
import type { Node, TokenResponse } from '../types';
import { Copy, Check } from 'lucide-react';

export default function Enrollment() {
  const [nodes, setNodes] = useState<Node[]>([]);
  const [parentId, setParentId] = useState('');
  const [nodeName, setNodeName] = useState('');
  const [ttl, setTtl] = useState(600);
  const [result, setResult] = useState<TokenResponse | null>(null);
  const [copied, setCopied] = useState(false);

  useEffect(() => { api.getNodes().then(setNodes).catch(() => {}); }, []);

  const generate = async () => {
    try {
      const res = await api.createToken({ parent_node_id: parentId, suggested_node_name: nodeName, ttl_seconds: ttl });
      setResult(res);
    } catch (e) { alert('生成失败: ' + e); }
  };

  const copy = async (text: string) => {
    await navigator.clipboard.writeText(text);
    setCopied(true); setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div>
      <h2 style={{ fontSize: 22, fontWeight: 600, marginBottom: 16 }}>接入新节点</h2>
      <div style={{ maxWidth: 560 }}>
        <div style={{ background: '#fff', borderRadius: 10, padding: 24, boxShadow: '0 1px 3px rgba(0,0,0,0.08)', marginBottom: 16 }}>
          <label style={lbl}>选择父节点</label>
          <select value={parentId} onChange={e => setParentId(e.target.value)} style={inp}>
            <option value="">-- 选择已有节点作为父节点 --</option>
            {nodes.map(n => (
              <option key={n.id} value={n.id}>{n.display_name} ({n.overlay_ipv4}) {n.is_controller ? '[控制机]' : ''}</option>
            ))}
          </select>

          <label style={lbl}>新节点名称</label>
          <input value={nodeName} onChange={e => setNodeName(e.target.value)} placeholder="例如: tokyo-node" style={inp} />

          <label style={lbl}>Token 有效期 (秒)</label>
          <input type="number" value={ttl} onChange={e => setTtl(Number(e.target.value))} style={inp} />

          <button onClick={generate} disabled={!parentId || !nodeName} style={{
            width: '100%', padding: '10px', background: '#0f172a', color: '#fff', border: 'none',
            borderRadius: 8, fontSize: 14, cursor: 'pointer', marginTop: 8,
            opacity: (!parentId || !nodeName) ? 0.4 : 1,
          }}>生成安装命令</button>
        </div>

        {result && (
          <div style={{ background: '#fff', borderRadius: 10, padding: 24, boxShadow: '0 1px 3px rgba(0,0,0,0.08)' }}>
            <h3 style={{ fontSize: 15, fontWeight: 600, marginBottom: 12 }}>安装命令</h3>
            <p style={{ fontSize: 12, color: '#64748b', marginBottom: 8 }}>在新节点上运行以下命令：</p>
            <div style={{ background: '#0f172a', padding: 16, borderRadius: 8, position: 'relative' }}>
              <pre style={{ color: '#38bdf8', fontSize: 12, whiteSpace: 'pre-wrap', wordBreak: 'break-all', margin: 0, fontFamily: 'monospace' }}>
                {result.install_command}
              </pre>
              <button onClick={() => copy(result.install_command)} style={{
                position: 'absolute', top: 8, right: 8, background: 'rgba(255,255,255,0.1)',
                border: 'none', borderRadius: 4, color: '#fff', cursor: 'pointer',
                padding: '6px 10px', display: 'flex', alignItems: 'center', gap: 6, fontSize: 12,
              }}>
                {copied ? <Check size={14} /> : <Copy size={14} />}
                {copied ? '已复制' : '复制'}
              </button>
            </div>
            <p style={{ fontSize: 12, color: '#64748b', marginTop: 12 }}>
              有效期至: {new Date(result.expires_at).toLocaleString()}
            </p>
          </div>
        )}
      </div>
    </div>
  );
}

const lbl: React.CSSProperties = { display: 'block', fontSize: 13, color: '#475569', marginBottom: 4, marginTop: 12 };
const inp: React.CSSProperties = { width: '100%', padding: '8px 10px', border: '1px solid #cbd5e1', borderRadius: 6, fontSize: 13, boxSizing: 'border-box' };
