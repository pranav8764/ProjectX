'use client';

import React, { useMemo, useState } from 'react';
import NavigationShell from '../../components/NavigationShell';
import { useData } from '../../context/DataContext';
import { Activity, AlertTriangle, FileText, Network, ShieldAlert } from 'lucide-react';
import Link from 'next/link';

type GraphMode = 'all' | 'assets' | 'gaps';

export default function KnowledgeGraphPage() {
  const { assets, documents, complianceGaps } = useData();
  const [mode, setMode] = useState<GraphMode>('all');
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);

  const nodes = useMemo(() => {
    const assetNodes = assets.map((asset, index) => ({
      id: asset.assetTag,
      label: asset.assetTag,
      subtitle: asset.assetType,
      type: 'asset',
      x: 180,
      y: 90 + index * 92,
      risk: asset.riskScore,
    }));

    const documentNodes = documents.slice(0, 8).map((doc, index) => ({
      id: doc.id,
      label: doc.title.replace(/\.(pdf|xlsx|docx|csv)$/i, ''),
      subtitle: doc.documentType,
      type: 'document',
      x: 560,
      y: 70 + index * 66,
      risk: 0,
    }));

    const gapNodes = complianceGaps.map((gap, index) => ({
      id: gap.id,
      label: gap.gapType,
      subtitle: gap.assetTag,
      type: 'gap',
      x: 940,
      y: 100 + index * 94,
      risk: gap.severity === 'Critical' ? 95 : gap.severity === 'High' ? 76 : 45,
    }));

    if (mode === 'assets') return [...assetNodes, ...documentNodes];
    if (mode === 'gaps') return [...assetNodes, ...gapNodes];
    return [...assetNodes, ...documentNodes, ...gapNodes];
  }, [assets, complianceGaps, documents, mode]);

  const edges = useMemo(() => {
    const docEdges = assets.flatMap(asset =>
      documents
        .filter(doc => asset.documents.includes(doc.id) || doc.title.toUpperCase().includes(asset.assetTag))
        .map(doc => ({ from: asset.assetTag, to: doc.id, kind: 'document' }))
    );

    const gapEdges = complianceGaps.map(gap => ({
      from: gap.assetTag,
      to: gap.id,
      kind: 'gap',
    }));

    if (mode === 'assets') return docEdges;
    if (mode === 'gaps') return gapEdges;
    return [...docEdges, ...gapEdges];
  }, [assets, complianceGaps, documents, mode]);

  const nodeById = new Map(nodes.map(node => [node.id, node]));
  const selectedNode = selectedNodeId ? nodeById.get(selectedNodeId) : null;
  const selectedEdgeCount = selectedNode
    ? edges.filter(edge => edge.from === selectedNode.id || edge.to === selectedNode.id).length
    : 0;

  return (
    <NavigationShell>
      <div className="space-y-6">
        <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
          <Metric label="Assets" value={assets.length} icon={<Network className="w-5 h-5" />} />
          <Metric label="Documents" value={documents.length} icon={<FileText className="w-5 h-5" />} />
          <Metric label="Open Gaps" value={complianceGaps.filter(g => g.status === 'Open').length} icon={<ShieldAlert className="w-5 h-5" />} />
          <Metric label="High Risk" value={assets.filter(a => a.riskScore >= 70).length} icon={<AlertTriangle className="w-5 h-5" />} />
        </div>

        <div className="glass-panel rounded-2xl border-slate-800/80 overflow-hidden">
          <div className="p-5 border-b border-slate-800/80 flex flex-col md:flex-row md:items-center justify-between gap-4">
            <div>
              <h3 className="text-base font-bold text-white tracking-wide">Asset Knowledge Graph</h3>
              <p className="text-xs text-slate-400 mt-0.5">Assets, evidence documents, and compliance gaps</p>
            </div>

            <div className="flex bg-slate-900/70 border border-slate-800 rounded-xl p-1 text-xs font-semibold">
              {[
                ['all', 'All'],
                ['assets', 'Documents'],
                ['gaps', 'Gaps'],
              ].map(([value, label]) => (
                <button
                  key={value}
                  onClick={() => setMode(value as GraphMode)}
                  className={`px-3 py-1.5 rounded-lg transition-all ${
                    mode === value ? 'bg-cyber-emerald text-slate-950' : 'text-slate-400 hover:text-white'
                  }`}
                >
                  {label}
                </button>
              ))}
            </div>
          </div>

          <div className="overflow-x-auto">
            <svg viewBox="0 0 1160 620" className="min-w-[980px] w-full h-[620px] bg-[#060918]">
              <defs>
                <linearGradient id="edgeGradient" x1="0" x2="1">
                  <stop offset="0%" stopColor="#10b981" stopOpacity="0.75" />
                  <stop offset="100%" stopColor="#38bdf8" stopOpacity="0.35" />
                </linearGradient>
              </defs>

              {edges.map((edge, index) => {
                const from = nodeById.get(edge.from);
                const to = nodeById.get(edge.to);
                if (!from || !to) return null;
                const midX = (from.x + to.x) / 2;
                return (
                  <path
                    key={`${edge.from}-${edge.to}-${index}`}
                    d={`M ${from.x + 82} ${from.y} C ${midX} ${from.y}, ${midX} ${to.y}, ${to.x - 82} ${to.y}`}
                    fill="none"
                    stroke={edge.kind === 'gap' ? '#f43f5e' : 'url(#edgeGradient)'}
                    strokeOpacity="0.55"
                    strokeWidth="2"
                  />
                );
              })}

              {nodes.map(node => {
                const isAsset = node.type === 'asset';
                const isGap = node.type === 'gap';
                const isSelected = selectedNodeId === node.id;
                return (
                  <g
                    key={node.id}
                    transform={`translate(${node.x - 82}, ${node.y - 31})`}
                    onClick={() => setSelectedNodeId(node.id)}
                    className="cursor-pointer"
                  >
                    <rect
                      width="164"
                      height="62"
                      rx="8"
                      fill={isAsset ? '#0f172a' : isGap ? '#1f0b14' : '#08111f'}
                      stroke={isAsset ? '#10b981' : isGap ? '#f43f5e' : '#38bdf8'}
                      strokeOpacity={isSelected ? '0.95' : '0.45'}
                      strokeWidth={isSelected ? '3' : '1'}
                    />
                    <text x="14" y="25" fill="#f8fafc" fontSize="13" fontWeight="700">
                      {node.label.length > 22 ? `${node.label.slice(0, 21)}...` : node.label}
                    </text>
                    <text x="14" y="45" fill="#94a3b8" fontSize="10">
                      {node.subtitle}
                    </text>
                    {isAsset && (
                      <text x="122" y="45" fill={node.risk >= 70 ? '#fb7185' : node.risk >= 40 ? '#fbbf24' : '#34d399'} fontSize="10" fontWeight="700">
                        {node.risk}
                      </text>
                    )}
                  </g>
                );
              })}
            </svg>
          </div>

          {selectedNode && (
            <div className="border-t border-slate-800/80 p-5 flex flex-col md:flex-row md:items-center justify-between gap-4 bg-[#090d1f]/60">
              <div>
                <span className="text-[10px] text-slate-500 font-mono uppercase tracking-wider">{selectedNode.type} node</span>
                <h4 className="text-sm font-bold text-white mt-1">{selectedNode.label}</h4>
                <p className="text-xs text-slate-400 mt-1">{selectedNode.subtitle} / {selectedEdgeCount} graph relationships visible</p>
              </div>
              {selectedNode.type === 'asset' && (
                <Link
                  href={`/assets/${selectedNode.id}`}
                  className="px-3 py-2 bg-slate-900 border border-slate-800 hover:border-cyber-emerald/30 text-slate-300 hover:text-cyber-emerald font-semibold rounded-xl text-xs transition-all"
                >
                  Open Asset Profile
                </Link>
              )}
            </div>
          )}
        </div>

        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          {assets
            .filter(asset => asset.riskScore >= 60 || asset.complianceGaps.length > 0)
            .slice(0, 4)
            .map(asset => (
              <Link
                key={asset.id}
                href={`/assets/${asset.assetTag}`}
                className="glass-panel p-4 rounded-xl border-slate-800/80 hover:border-cyber-emerald/30 transition-all flex items-center justify-between gap-4"
              >
                <div>
                  <span className="text-xs font-mono text-cyber-emerald font-bold">{asset.assetTag}</span>
                  <h4 className="text-sm font-bold text-white mt-1">{asset.assetName}</h4>
                  <p className="text-xs text-slate-400 mt-0.5">{asset.failures.length} failures / {asset.documents.length} references</p>
                </div>
                <div className="flex items-center gap-2 text-xs font-mono">
                  <Activity className="w-4 h-4 text-cyber-amber" />
                  <span className="text-slate-300">{asset.riskScore}</span>
                </div>
              </Link>
            ))}
        </div>
      </div>
    </NavigationShell>
  );
}

function Metric({ label, value, icon }: { label: string; value: number; icon: React.ReactNode }) {
  return (
    <div className="glass-panel p-4 rounded-xl flex items-center gap-4">
      <div className="p-3 bg-emerald-500/10 text-cyber-emerald rounded-lg border border-emerald-500/20">
        {icon}
      </div>
      <div>
        <span className="text-[10px] text-slate-500 font-mono uppercase tracking-wider block">{label}</span>
        <span className="text-xl font-bold text-white">{value}</span>
      </div>
    </div>
  );
}
