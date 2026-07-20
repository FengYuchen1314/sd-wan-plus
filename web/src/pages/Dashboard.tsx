import React, { useEffect, useState } from 'react';
import { api } from '../api';
import { Server, Share2, FileText, Wifi, WifiOff, Activity } from 'lucide-react';

export default function Dashboard() {
  const [nodes, setNodes] = useState<any[]>([]);
  const [links, setLinks] = useState<any[]>([]);
  const [policies, setPolicies] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    api.getNodes().then(n => { setNodes(Array.isArray(n) ? n : []); }).catch(() => {});
    api.getLinks().then(l => { setLinks(Array.isArray(l) ? l : []); }).catch(() => {});
    api.getPolicies().then(p => { setPolicies(Array.isArray(p) ? p : []); }).catch(() => {});
    setLoading(false);
  }, []);

  const onlineCount = nodes.filter(n => n.last_seen_at).length;
  const activeLinks = links.filter(l => l.enabled && l.status === 'Active').length;

  const stats = [
    { label: '节点总数', value: nodes.length },
    { label: '在线节点', value: onlineCount },
    { label: 'WG 链路', value: links.length },
    { label: '活跃链路', value: activeLinks },
    { label: '策略数量', value: policies.length },
    { label: '离线节点', value: nodes.length - onlineCount },
  ];

  const icons = [Server, Wifi, Share2, Activity, FileText, WifiOff];
  const colors = ['#3b82f6', '#22c55e', '#8b5cf6', '#0ea5e9', '#f59e0b', '#ef4444'];

  if (loading) return <div style={{ padding: 24, color: '#64748b' }}>加载中...</div>;

  return (
    <div>
      <h2 style={{ fontSize: 22, fontWeight: 600, marginBottom: 20 }}>仪表盘</h2>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(200px, 1fr))', gap: 16 }}>
        {stats.map(({ label, value }, i) => (
          <div key={label} style={{
            background: '#fff', borderRadius: 10, padding: 20, boxShadow: '0 1px 3px rgba(0,0,0,0.08)',
          }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'start' }}>
              <div>
                <p style={{ fontSize: 13, color: '#64748b', marginBottom: 4 }}>{label}</p>
                <p style={{ fontSize: 28, fontWeight: 700 }}>{value}</p>
              </div>
              {React.createElement(icons[i], { size: 20, color: colors[i] })}
            </div>
          </div>
        ))}
      </div>

      <h3 style={{ fontSize: 16, fontWeight: 600, marginTop: 28, marginBottom: 12 }}>节点列表</h3>
      <div style={{ background: '#fff', borderRadius: 10, overflow: 'hidden', boxShadow: '0 1px 3px rgba(0,0,0,0.08)' }}>
        <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
          <thead>
            <tr style={{ background: '#f8fafc', borderBottom: '1px solid #e2e8f0' }}>
              <th style={{ padding: '10px 16px', textAlign: 'left', color: '#64748b', fontWeight: 600, fontSize: 12 }}>名称</th>
              <th style={{ padding: '10px 16px', textAlign: 'left', color: '#64748b', fontWeight: 600, fontSize: 12 }}>Overlay IP</th>
              <th style={{ padding: '10px 16px', textAlign: 'left', color: '#64748b', fontWeight: 600, fontSize: 12 }}>类型</th>
              <th style={{ padding: '10px 16px', textAlign: 'left', color: '#64748b', fontWeight: 600, fontSize: 12 }}>状态</th>
            </tr>
          </thead>
          <tbody>
            {nodes.map((n: any) => (
              <tr key={n.id} style={{ borderBottom: '1px solid #f1f5f9' }}>
                <td style={{ padding: '10px 16px' }}>{n.display_name}</td>
                <td style={{ padding: '10px 16px', fontFamily: 'monospace', fontSize: 12 }}>{n.overlay_ipv4}</td>
                <td style={{ padding: '10px 16px' }}>
                  <span style={{ padding: '2px 8px', borderRadius: 10, fontSize: 11,
                    background: n.is_controller ? '#fef3c7' : '#e0f2fe',
                    color: n.is_controller ? '#92400e' : '#0369a1',
                  }}>{n.is_controller ? '控制机' : '节点'}</span>
                </td>
                <td style={{ padding: '10px 16px' }}>
                  <span style={{ color: n.last_seen_at ? '#16a34a' : '#94a3b8', fontSize: 12 }}>
                    {n.last_seen_at ? '在线' : '离线'}
                  </span>
                </td>
              </tr>
            ))}
            {nodes.length === 0 && (
              <tr><td colSpan={4} style={{ padding: 24, textAlign: 'center', color: '#94a3b8' }}>暂无节点接入 — 请先在"接入新节点"页面生成安装命令</td></tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
