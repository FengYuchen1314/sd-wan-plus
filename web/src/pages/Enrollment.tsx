import { useEffect, useState } from 'react';
import { api } from '../api';
import type { Node, TokenResponse } from '../types';
import { Copy, Check, Clock, Shield } from 'lucide-react';
import React from 'react';

export default function Enrollment() {
  const [nodes, setNodes] = useState<Node[]>([]);
  const [parentId, setParentId] = useState('');
  const [nodeName, setNodeName] = useState('');
  const [ttl, setTtl] = useState(600);
  const [result, setResult] = useState<TokenResponse | null>(null);
  const [copied, setCopied] = useState(false);
  const [tokens, setTokens] = useState<any[]>([]);

  useEffect(() => {
    api.getNodes().then(setNodes).catch(() => {});
    api.getTokens().then(setTokens).catch(() => {});
  }, []);

  const generate = async () => {
    try {
      const res = await api.createToken({ parent_node_id: parentId, suggested_node_name: nodeName, ttl_seconds: ttl });
      setResult(res);
      api.getTokens().then(setTokens).catch(() => {});
    } catch (e: any) { alert('生成失败: ' + e.message); }
  };

  const revokeToken = async (id: string) => {
    try { await api.revokeToken(id); api.getTokens().then(setTokens).catch(() => {}); alert('Token 已撤销'); }
    catch (e: any) { alert('撤销失败: ' + e.message); }
  };

  const copy = async (text: string) => {
    await navigator.clipboard.writeText(text);
    setCopied(true); setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div>
      <h2 style={{ fontSize: 22, fontWeight: 600, marginBottom: 16 }}>接入新节点</h2>
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16, alignItems: 'start' }}>
        <div>
          <div style={{ background: '#fff', borderRadius: 10, padding: 24, boxShadow: '0 1px 3px rgba(0,0,0,0.06)' }}>
            <label style={lbl}>选择父节点</label>
            <select value={parentId} onChange={e => setParentId(e.target.value)} style={inp}>
              <option value="">-- 选择已有节点作为父节点 --</option>
              {nodes.map(n => (
                <option key={n.id} value={n.id}>
                  {n.display_name} ({n.overlay_ipv4}) {n.is_controller ? '[控制机]' : ''}
                </option>
              ))}
            </select>

            <label style={lbl}>新节点名称</label>
            <input value={nodeName} onChange={e => setNodeName(e.target.value)} placeholder="例如: tokyo-node" style={inp} />

            <label style={lbl}>Token 有效期 (秒, 10-86400)</label>
            <input type="number" value={ttl} onChange={e => setTtl(Math.max(10, Math.min(86400, Number(e.target.value))))}
              min={10} max={86400} style={inp} />

            <button onClick={generate} disabled={!parentId || !nodeName} style={{
              width: '100%', padding: '10px', background: '#0f172a', color: '#fff', border: 'none',
              borderRadius: 8, fontSize: 14, cursor: 'pointer', marginTop: 12, fontWeight: 600,
              opacity: (!parentId || !nodeName) ? 0.4 : 1,
            }}>生成安装命令</button>

            <div style={{ marginTop: 12, padding: '8px 12px', background: '#eff6ff', borderRadius: 6, fontSize: 11, color: '#2563eb', display: 'flex', alignItems: 'flex-start', gap: 6 }}>
              <Shield size={14} style={{ flexShrink: 0, marginTop: 1 }} />
              <span>Token 一次性有效，绑定指定父节点，过期自动失效。安装命令中的地址是父节点的可达地址。</span>
            </div>
          </div>

          {result && (
            <div style={{ background: '#fff', borderRadius: 10, padding: 24, marginTop: 16, boxShadow: '0 1px 3px rgba(0,0,0,0.06)' }}>
              <h3 style={{ fontSize: 15, fontWeight: 600, marginBottom: 12 }}>安装命令</h3>
              <p style={{ fontSize: 12, color: '#64748b', marginBottom: 8 }}>
                在目标机器 (Linux, systemd) 上运行：
              </p>
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
              <div style={{ marginTop: 12, display: 'flex', gap: 16, fontSize: 12, color: '#64748b' }}>
                <span style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
                  <Clock size={14} /> 有效期至: {new Date(result.expires_at).toLocaleString()}
                </span>
                <span>父节点: {result.parent_node_name}</span>
              </div>
            </div>
          )}
        </div>

        <div>
          <div style={{ background: '#fff', borderRadius: 10, padding: 20, boxShadow: '0 1px 3px rgba(0,0,0,0.06)' }}>
            <h3 style={{ fontSize: 14, fontWeight: 600, marginBottom: 12 }}>活跃 Token</h3>
            {tokens.length === 0 && (
              <p style={{ fontSize: 13, color: '#94a3b8', textAlign: 'center', padding: 20 }}>暂无活跃 Token</p>
            )}
            {tokens.filter((t: any) => !t.used_at && !t.revoked).map((t: any) => (
              <div key={t.id} style={{ padding: '10px 0', borderBottom: '1px solid #f1f5f9', fontSize: 12 }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                  <span style={{ fontWeight: 500 }}>{t.suggested_node_name}</span>
                  <span style={{ fontSize: 10, color: t.expires_at < new Date().toISOString() ? '#dc2626' : '#64748b' }}>
                    {new Date(t.expires_at).toLocaleString()}
                  </span>
                </div>
                <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: 4 }}>
                  <code style={{ fontSize: 10, color: '#94a3b8' }}>{t.token.slice(0, 16)}...</code>
                  <button onClick={() => revokeToken(t.id)}
                    style={{ fontSize: 11, color: '#dc2626', background: 'none', border: 'none', cursor: 'pointer', fontWeight: 500 }}>
                    撤销
                  </button>
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}

const lbl: React.CSSProperties = { display: 'block', fontSize: 13, color: '#475569', marginBottom: 4, marginTop: 14, fontWeight: 500 };
const inp: React.CSSProperties = { width: '100%', padding: '8px 10px', border: '1px solid #cbd5e1', borderRadius: 6, fontSize: 13, boxSizing: 'border-box' };
