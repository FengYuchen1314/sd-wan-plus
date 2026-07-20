import { useEffect, useState } from 'react';
import { api } from '../api';
import type { Node, WireGuardLink, PolicyDetail } from '../types';
import { Server, Share2, FileText, Wifi, WifiOff, Activity, ShieldCheck } from 'lucide-react';
import React from 'react';

export default function Dashboard() {
  const [nodes, setNodes] = useState<Node[]>([]);
  const [links, setLinks] = useState<WireGuardLink[]>([]);
  const [policies, setPolicies] = useState<PolicyDetail[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    Promise.all([
      api.getNodes().catch(() => []),
      api.getLinks().catch(() => []),
      api.getPolicies().catch(() => []),
    ]).then(([n, l, p]) => {
      setNodes(n);
      setLinks(l);
      setPolicies(p);
      setLoading(false);
    });
  }, []);

  const onlineAgents = nodes.filter(n => n.last_seen_at).length;
  const activeLinks = links.filter(l => l.enabled && l.status === 'Active').length;
  const failedLinks = links.filter(l => l.status !== 'Active').length;
  const configConsistent = nodes.filter(n => n.desired_generation === n.active_generation).length;
  const activePolicies = policies.filter(p => p.policy.enabled).length;

  const stats = [
    { label: '节点总数', value: nodes.length, icon: Server, color: '#3b82f6' },
    { label: 'Agent在线', value: onlineAgents, icon: Wifi, color: '#22c55e' },
    { label: 'WG链路', value: links.length, icon: Share2, color: '#8b5cf6' },
    { label: '活跃链路', value: activeLinks, icon: Activity, color: '#0ea5e9' },
    { label: '策略数量', value: activePolicies, icon: FileText, color: '#f59e0b' },
    { label: '离线节点', value: nodes.length - onlineAgents, icon: WifiOff, color: '#ef4444' },
    { label: '配置一致', value: configConsistent, icon: ShieldCheck, color: '#06b6d4' },
    { label: '故障链路', value: failedLinks, icon: WifiOff, color: '#dc2626' },
  ];

  if (loading) return (
    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: 200, color: '#64748b', fontSize: 14 }}>
      加载中...
    </div>
  );

  return (
    <div>
      <h2 style={{ fontSize: 22, fontWeight: 600, marginBottom: 20 }}>仪表盘</h2>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(200px, 1fr))', gap: 14 }}>
        {stats.map(({ label, value, icon: Icon, color }) => (
          <div key={label} style={{
            background: '#fff', borderRadius: 10, padding: 18, boxShadow: '0 1px 3px rgba(0,0,0,0.06)',
          }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'start' }}>
              <div>
                <p style={{ fontSize: 12, color: '#64748b', marginBottom: 4 }}>{label}</p>
                <p style={{ fontSize: 26, fontWeight: 700, color: '#0f172a' }}>{value}</p>
              </div>
              {React.createElement(Icon, { size: 20, color })}
            </div>
          </div>
        ))}
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16, marginTop: 24 }}>
        <div>
          <h3 style={{ fontSize: 15, fontWeight: 600, marginBottom: 10 }}>节点状态</h3>
          <div style={{ background: '#fff', borderRadius: 10, overflow: 'hidden', boxShadow: '0 1px 3px rgba(0,0,0,0.06)' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
              <thead>
                <tr style={{ background: '#f8fafc', borderBottom: '1px solid #e2e8f0' }}>
                  <th style={th}>名称</th><th style={th}>Overlay IP</th><th style={th}>类型</th><th style={th}>Agent</th><th style={th}>配置</th>
                </tr>
              </thead>
              <tbody>
                {nodes.map(n => (
                  <tr key={n.id} style={{ borderBottom: '1px solid #f1f5f9' }}>
                    <td style={td}>{n.display_name}</td>
                    <td style={{ ...td, fontFamily: 'monospace', fontSize: 12 }}>{n.overlay_ipv4}</td>
                    <td style={td}>
                      <span style={{ ...badge, background: n.is_controller ? '#fef3c7' : '#e0f2fe', color: n.is_controller ? '#92400e' : '#0369a1' }}>
                        {n.is_controller ? '控制机' : '节点'}
                      </span>
                    </td>
                    <td style={td}>
                      <span style={{ color: n.last_seen_at ? '#16a34a' : '#94a3b8', fontSize: 12 }}>
                        {n.last_seen_at ? '在线' : '离线'}
                      </span>
                    </td>
                    <td style={td}>
                      <span style={{ color: n.desired_generation === n.active_generation ? '#16a34a' : '#f59e0b', fontSize: 12 }}>
                        {n.desired_generation === n.active_generation ? '一致' : '待同步'}
                      </span>
                    </td>
                  </tr>
                ))}
                {nodes.length === 0 && (
                  <tr><td colSpan={5} style={{ padding: 20, textAlign: 'center', color: '#94a3b8', fontSize: 13 }}>暂无节点 — 请先在"接入新节点"页面生成安装命令</td></tr>
                )}
              </tbody>
            </table>
          </div>
        </div>

        <div>
          <h3 style={{ fontSize: 15, fontWeight: 600, marginBottom: 10 }}>WireGuard 链路</h3>
          <div style={{ background: '#fff', borderRadius: 10, overflow: 'hidden', boxShadow: '0 1px 3px rgba(0,0,0,0.06)' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
              <thead>
                <tr style={{ background: '#f8fafc', borderBottom: '1px solid #e2e8f0' }}>
                  <th style={th}>链路</th><th style={th}>接口 A</th><th style={th}>接口 B</th><th style={th}>状态</th>
                </tr>
              </thead>
              <tbody>
                {links.map(l => (
                  <tr key={l.id} style={{ borderBottom: '1px solid #f1f5f9' }}>
                    <td style={td}>{l.node_a_name} ↔ {l.node_b_name}</td>
                    <td style={{ ...td, fontFamily: 'monospace', fontSize: 11 }}>{l.interface_name_a}</td>
                    <td style={{ ...td, fontFamily: 'monospace', fontSize: 11 }}>{l.interface_name_b}</td>
                    <td style={td}>
                      <span style={{ ...badge, background: l.status === 'Active' ? '#dcfce7' : '#fef2f2', color: l.status === 'Active' ? '#16a34a' : '#dc2626' }}>
                        {l.status}
                      </span>
                    </td>
                  </tr>
                ))}
                {links.length === 0 && (
                  <tr><td colSpan={4} style={{ padding: 20, textAlign: 'center', color: '#94a3b8', fontSize: 13 }}>暂无链路</td></tr>
                )}
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>
  );
}

const th: React.CSSProperties = { padding: '10px 14px', textAlign: 'left', color: '#64748b', fontWeight: 600, fontSize: 11, textTransform: 'uppercase' };
const td: React.CSSProperties = { padding: '10px 14px' };
const badge: React.CSSProperties = { padding: '2px 8px', borderRadius: 10, fontSize: 11, fontWeight: 500 };
