import { useEffect, useRef, useState } from 'react';
import { api } from '../api';
import type { Node, WireGuardLink } from '../types';

export default function Topology() {
  const container = useRef<HTMLDivElement>(null);
  const [nodes, setNodes] = useState<Node[]>([]);
  const [links, setLinks] = useState<WireGuardLink[]>([]);
  const [selectedA, setSelectedA] = useState<string | null>(null);
  const [selectedB, setSelectedB] = useState<string | null>(null);
  const [creatingLink, setCreatingLink] = useState(false);

  useEffect(() => {
    Promise.all([
      api.getNodes().catch(() => []),
      api.getLinks().catch(() => []),
    ]).then(([n, l]) => { setNodes(n); setLinks(l); });
  }, []);

  useEffect(() => {
    if (!container.current || nodes.length === 0) return;

    const init = async () => {
      const cytoscape = (await import('cytoscape')).default;
      const dagre = (await import('cytoscape-dagre')).default;
      cytoscape.use(dagre);

      const cyNodes = nodes.map(n => ({
        data: { id: n.id, label: n.display_name, ip: n.overlay_ipv4, isController: n.is_controller },
      }));

      const cyEdges = links.filter(l => l.enabled).map(l => ({
        data: { id: l.id, source: l.node_a, target: l.node_b, label: l.interface_name_a.replace('pwl-', '') },
      }));

      const cy = cytoscape({
        container: container.current!,
        elements: [...cyNodes, ...cyEdges],
        style: [
          {
            selector: 'node',
            style: {
              label: 'data(label)', 'background-color': '#3b82f6', color: '#fff',
              'font-size': 11, 'text-valign': 'center', 'text-halign': 'center',
              width: 50, height: 50, 'border-width': 2, 'border-color': '#2563eb',
            },
          },
          {
            selector: 'node[isController=true]',
            style: { 'background-color': '#f59e0b', 'border-color': '#d97706', width: 60, height: 60 },
          },
          {
            selector: 'edge',
            style: {
              width: 2, 'line-color': '#94a3b8', 'target-arrow-color': '#94a3b8',
              'target-arrow-shape': 'triangle', 'curve-style': 'bezier',
              label: 'data(label)', 'font-size': 9, color: '#64748b',
            },
          },
        ],
        layout: { name: 'dagre', rankDir: 'TB', spacingFactor: 1.3 },
        wheelSensitivity: 0.3,
      });

      cy.on('tap', 'node', (evt) => {
        const id = evt.target.id();
        if (!selectedA) { setSelectedA(id); }
        else if (!selectedB && id !== selectedA) { setSelectedB(id); }
        else { setSelectedA(id); setSelectedB(null); }
      });

      cy.on('tap', (evt) => {
        if (evt.target === cy) { setSelectedA(null); setSelectedB(null); }
      });
    };

    init();
  }, [nodes, links]);

  const handleCreateLink = async () => {
    if (!selectedA || !selectedB) return;
    setCreatingLink(true);
    try {
      const b = nodes.find(n => n.id === selectedB);
      await api.createLink({
        initiator_node_id: selectedA,
        listener_node_id: selectedB,
        listener_address: b?.overlay_ipv4 || '10.250.0.1',
      });
      alert('链路创建成功');
      const [n, l] = await Promise.all([api.getNodes(), api.getLinks()]);
      setNodes(n); setLinks(l);
    } catch (e) {
      alert('创建失败: ' + e);
    } finally {
      setCreatingLink(false);
    }
  };

  return (
    <div>
      <h2 style={{ fontSize: 22, fontWeight: 600, marginBottom: 16 }}>网络拓扑图</h2>
      <div style={{
        display: 'flex', gap: 12, marginBottom: 16, alignItems: 'center',
        background: '#fff', padding: 12, borderRadius: 10, boxShadow: '0 1px 3px rgba(0,0,0,0.08)',
      }}>
        <span style={{ fontSize: 13, color: '#64748b' }}>选择两个节点来创建 WireGuard 链路</span>
        {selectedA && <span style={{
          padding: '4px 10px', borderRadius: 6, background: '#e0f2fe', fontSize: 13, color: '#0369a1',
        }}>主动端: {nodes.find(n => n.id === selectedA)?.display_name || selectedA}</span>}
        {selectedB && <span style={{
          padding: '4px 10px', borderRadius: 6, background: '#fef3c7', fontSize: 13, color: '#92400e',
        }}>被动端: {nodes.find(n => n.id === selectedB)?.display_name || selectedB}</span>}
        <button onClick={handleCreateLink} disabled={!selectedA || !selectedB || creatingLink}
          style={{
            padding: '8px 16px', background: '#0f172a', color: '#fff', border: 'none',
            borderRadius: 6, cursor: 'pointer', fontSize: 13,
            opacity: (!selectedA || !selectedB) ? 0.4 : 1,
          }}>
          {creatingLink ? '创建中...' : '建立链路'}
        </button>
      </div>
      <div ref={container} style={{
        width: '100%', height: 500, background: '#fff',
        borderRadius: 10, boxShadow: '0 1px 3px rgba(0,0,0,0.08)',
      }} />
    </div>
  );
}
