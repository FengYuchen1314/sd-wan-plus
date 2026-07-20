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
  const [showControlTree, setShowControlTree] = useState(true);
  const [showDataGraph, setShowDataGraph] = useState(true);
  const [listenerAddr, setListenerAddr] = useState('');

  const load = async () => {
    const [n, l] = await Promise.all([api.getNodes().catch(() => []), api.getLinks().catch(() => [])]);
    setNodes(n); setLinks(l);
  };

  useEffect(() => { load(); }, []);

  useEffect(() => {
    if (!container.current || nodes.length === 0) return;
    const init = async () => {
      const cytoscape = (await import('cytoscape')).default;
      const dagre = (await import('cytoscape-dagre')).default;
      cytoscape.use(dagre);

      const cyNodes = nodes.map(n => ({
        data: {
          id: n.id, label: n.display_name, ip: n.overlay_ipv4,
          isController: n.is_controller, controlParent: n.control_parent_id,
        },
      }));

      const cyEdges: any[] = [];

      // Control tree edges (dashed style)
      if (showControlTree) {
        nodes.filter(n => n.control_parent_id).forEach(n => {
          cyEdges.push({
            data: {
              id: `ctrl-${n.id}`,
              source: n.control_parent_id,
              target: n.id,
              edgeType: 'control',
              label: '控制',
            },
          });
        });
      }

      // Data graph edges (solid)
      if (showDataGraph) {
        links.filter(l => l.enabled).forEach(l => {
          cyEdges.push({
            data: {
              id: l.id,
              source: l.node_a,
              target: l.node_b,
              edgeType: 'data',
              label: l.interface_name_a?.replace('pwl-', ''),
            },
          });
        });
      }

      container.current!.innerHTML = '';
      const cy = cytoscape({
        container: container.current!,
        elements: [...cyNodes, ...cyEdges],
        style: [
          {
            selector: 'node',
            style: {
              label: 'data(label)',
              'background-color': '#3b82f6',
              color: '#fff',
              'font-size': 11,
              'text-valign': 'center',
              'text-halign': 'center',
              width: 55,
              height: 55,
              'border-width': 2,
              'border-color': '#2563eb',
              'text-wrap': 'wrap',
              'text-max-width': 80,
            },
          },
          {
            selector: 'node[isController=true]',
            style: { 'background-color': '#f59e0b', 'border-color': '#d97706', width: 65, height: 65 },
          },
          {
            selector: 'edge[edgeType="data"]',
            style: {
              width: 2.5,
              'line-color': '#94a3b8',
              'target-arrow-color': '#94a3b8',
              'target-arrow-shape': 'triangle',
              'curve-style': 'bezier',
              label: 'data(label)',
              'font-size': 9,
              color: '#64748b',
            },
          },
          {
            selector: 'edge[edgeType="control"]',
            style: {
              width: 1.5,
              'line-color': '#cbd5e1',
              'line-style': 'dashed',
              'target-arrow-color': '#cbd5e1',
              'target-arrow-shape': 'triangle',
              'curve-style': 'bezier',
              label: 'data(label)',
              'font-size': 9,
              color: '#94a3b8',
            },
          },
        ],
        layout: { name: 'dagre', rankDir: 'TB', spacingFactor: 1.3 } as any,
        wheelSensitivity: 0.3,
      });

      cy.on('tap', 'node', (evt: any) => {
        const id = evt.target.id();
        if (!selectedA) { setSelectedA(id); }
        else if (!selectedB && id !== selectedA) { setSelectedB(id); }
        else { setSelectedA(id); setSelectedB(null); }
      });

      cy.on('tap', (evt: any) => {
        if (evt.target === cy) { setSelectedA(null); setSelectedB(null); }
      });
    };

    init();
  }, [nodes, links, showControlTree, showDataGraph]);

  const handleCreateLink = async () => {
    if (!selectedA || !selectedB) return;
    setCreatingLink(true);
    try {
      const b = nodes.find(n => n.id === selectedB);
      await api.createLink({
        initiator_node_id: selectedA,
        listener_node_id: selectedB,
        listener_address: listenerAddr || b?.overlay_ipv4 || '10.250.0.1',
      });
      alert('链路创建成功');
      await load();
      setSelectedA(null); setSelectedB(null); setListenerAddr('');
    } catch (e: any) {
      alert('创建失败: ' + e.message);
    } finally {
      setCreatingLink(false);
    }
  };

  return (
    <div>
      <h2 style={{ fontSize: 22, fontWeight: 600, marginBottom: 16 }}>网络拓扑图</h2>

      <div style={{
        display: 'flex', gap: 12, marginBottom: 14, flexWrap: 'wrap', alignItems: 'center',
        background: '#fff', padding: '12px 16px', borderRadius: 10, boxShadow: '0 1px 3px rgba(0,0,0,0.06)',
      }}>
        <label style={{ fontSize: 12, display: 'flex', alignItems: 'center', gap: 6, color: '#475569' }}>
          <input type="checkbox" checked={showControlTree} onChange={e => setShowControlTree(e.target.checked)} />
          <span style={{ borderBottom: '2px dashed #cbd5e1', paddingBottom: 2 }}>控制树</span>
        </label>
        <label style={{ fontSize: 12, display: 'flex', alignItems: 'center', gap: 6, color: '#475569' }}>
          <input type="checkbox" checked={showDataGraph} onChange={e => setShowDataGraph(e.target.checked)} />
          <span style={{ borderBottom: '2px solid #94a3b8', paddingBottom: 2 }}>数据链路</span>
        </label>
        <span style={{ color: '#cbd5e1' }}>|</span>
        <span style={{ fontSize: 13, color: '#64748b' }}>选择两个节点建立链路：</span>
        {selectedA && (
          <span style={{ ...selBadge, background: '#e0f2fe', color: '#0369a1' }}>
            主动端: {nodes.find(n => n.id === selectedA)?.display_name || selectedA}
          </span>
        )}
        {selectedB && (
          <span style={{ ...selBadge, background: '#fef3c7', color: '#92400e' }}>
            被动端: {nodes.find(n => n.id === selectedB)?.display_name || selectedB}
          </span>
        )}
        {selectedA && selectedB && (
          <input value={listenerAddr} onChange={e => setListenerAddr(e.target.value)}
            placeholder="被动端可达地址" style={{ padding: '4px 8px', border: '1px solid #cbd5e1', borderRadius: 4, fontSize: 12, width: 140 }} />
        )}
        <button onClick={handleCreateLink} disabled={!selectedA || !selectedB || creatingLink}
          style={{
            padding: '7px 14px', background: '#0f172a', color: '#fff', border: 'none',
            borderRadius: 6, cursor: 'pointer', fontSize: 13,
            opacity: (!selectedA || !selectedB) ? 0.4 : 1,
          }}>
          {creatingLink ? '创建中...' : '建立链路'}
        </button>
      </div>

      <div ref={container} style={{
        width: '100%', height: 520, background: '#fff',
        borderRadius: 10, boxShadow: '0 1px 3px rgba(0,0,0,0.06)',
      }} />

      <div style={{ marginTop: 12, fontSize: 11, color: '#94a3b8', display: 'flex', gap: 16 }}>
        <span><span style={{ display: 'inline-block', width: 20, borderBottom: '2px dashed #cbd5e1' }} /> 控制树</span>
        <span><span style={{ display: 'inline-block', width: 20, borderBottom: '2px solid #94a3b8' }} /> WireGuard 数据链路</span>
        <span><span style={{ display: 'inline-block', width: 14, height: 14, background: '#f59e0b', borderRadius: 3 }} /> 控制机</span>
        <span><span style={{ display: 'inline-block', width: 14, height: 14, background: '#3b82f6', borderRadius: 3 }} /> 普通节点</span>
      </div>
    </div>
  );
}

const selBadge: React.CSSProperties = { padding: '4px 10px', borderRadius: 6, fontSize: 12, fontWeight: 500 };
