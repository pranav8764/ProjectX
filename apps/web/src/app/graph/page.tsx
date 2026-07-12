'use client';

import React, { useMemo, useState, useEffect } from 'react';
import NavigationShell from '../../components/NavigationShell';
import { useData } from '../../context/DataContext';
import { apiFetch, appendPlantQuery, getStoredSession, PlantBrainSession } from '../../lib/api';
import { canRunAction } from '../../lib/permissions';
import { Activity, AlertTriangle, FileText, Network, Share2 } from 'lucide-react';
import Link from 'next/link';

interface ApiGraphNode {
  id: string;
  type: string;
  label: string;
  normalizedValue?: string;
  confidence?: number;
  documentId?: string | null;
  pageNo?: number | null;
}

interface ApiGraphEdge {
  id: string;
  source: string;
  target: string;
  type: string;
  confidence?: number;
}

interface PositionedNode extends ApiGraphNode {
  x: number;
  y: number;
  colorIndex: number;
  degree: number;
}

const VIEW_WIDTH = 1160;
const VIEW_HEIGHT = 700;
// Minimum centre-to-centre distance. Enforced as a hard constraint each iteration so
// nodes (and their labels) can never sit on top of each other.
const MIN_SEPARATION = 116;
const PAD_X = 90;
const PAD_Y = 46;

const TYPE_PALETTE = [
  { stroke: '#10b981', fill: '#052e23' }, // emerald
  { stroke: '#38bdf8', fill: '#082f44' }, // blue
  { stroke: '#a855f7', fill: '#2a1044' }, // violet
  { stroke: '#fbbf24', fill: '#3d2b06' }, // amber
  { stroke: '#f43f5e', fill: '#3f0d1c' }, // rose
  { stroke: '#22d3ee', fill: '#083944' }, // cyan
  { stroke: '#f97316', fill: '#43200a' }, // orange
  { stroke: '#84cc16', fill: '#26330a' }, // lime
  { stroke: '#e879f9', fill: '#3d0f42' }, // fuchsia
  { stroke: '#60a5fa', fill: '#10254d' }, // indigo
  { stroke: '#2dd4bf', fill: '#0a3b38' }, // teal
  { stroke: '#fb7185', fill: '#43111f' }, // pink
];

const truncate = (value: string, max: number) => (value.length > max ? `${value.slice(0, max - 1)}…` : value);

/**
 * Deterministic force-directed layout: repulsion + edge springs + centering, followed by
 * hard collision separation and bounds clamping. Seeded from a circle (no randomness) so
 * the graph renders identically on every load.
 */
function computeLayout(nodes: ApiGraphNode[], edges: ApiGraphEdge[]) {
  const count = nodes.length;
  const points = nodes.map((_, i) => {
    const angle = (2 * Math.PI * i) / Math.max(count, 1);
    return {
      x: VIEW_WIDTH / 2 + Math.cos(angle) * 270,
      y: VIEW_HEIGHT / 2 + Math.sin(angle) * 230,
      vx: 0,
      vy: 0,
    };
  });
  if (count === 0) return points;

  const indexById = new Map(nodes.map((node, i) => [node.id, i]));
  const links = edges
    .map(edge => ({ s: indexById.get(edge.source), t: indexById.get(edge.target) }))
    .filter((l): l is { s: number; t: number } => l.s !== undefined && l.t !== undefined && l.s !== l.t);

  const ITERATIONS = 420;
  for (let step = 0; step < ITERATIONS; step++) {
    const cooling = 1 - step / ITERATIONS;

    // Repulsion between every pair
    for (let i = 0; i < count; i++) {
      for (let j = i + 1; j < count; j++) {
        const dx = points[i].x - points[j].x;
        const dy = points[i].y - points[j].y;
        const distSq = Math.max(dx * dx + dy * dy, 1);
        const dist = Math.sqrt(distSq);
        const force = 14000 / distSq;
        const ux = dx / dist;
        const uy = dy / dist;
        points[i].vx += ux * force;
        points[i].vy += uy * force;
        points[j].vx -= ux * force;
        points[j].vy -= uy * force;
      }
    }

    // Springs along edges
    for (const link of links) {
      const a = points[link.s];
      const b = points[link.t];
      const dx = b.x - a.x;
      const dy = b.y - a.y;
      const dist = Math.max(Math.hypot(dx, dy), 0.01);
      const force = (dist - 190) * 0.015;
      const ux = dx / dist;
      const uy = dy / dist;
      a.vx += ux * force;
      a.vy += uy * force;
      b.vx -= ux * force;
      b.vy -= uy * force;
    }

    // Gentle pull to centre + integrate with damping
    for (let i = 0; i < count; i++) {
      points[i].vx += (VIEW_WIDTH / 2 - points[i].x) * 0.0022;
      points[i].vy += (VIEW_HEIGHT / 2 - points[i].y) * 0.0022;
      points[i].x += points[i].vx * 0.5 * cooling;
      points[i].y += points[i].vy * 0.5 * cooling;
      points[i].vx *= 0.82;
      points[i].vy *= 0.82;
    }

    // Hard separation — guarantees no overlapping nodes
    for (let i = 0; i < count; i++) {
      for (let j = i + 1; j < count; j++) {
        const dx = points[j].x - points[i].x;
        const dy = points[j].y - points[i].y;
        const dist = Math.max(Math.hypot(dx, dy), 0.01);
        if (dist < MIN_SEPARATION) {
          const push = (MIN_SEPARATION - dist) / 2;
          const ux = dx / dist;
          const uy = dy / dist;
          points[i].x -= ux * push;
          points[i].y -= uy * push;
          points[j].x += ux * push;
          points[j].y += uy * push;
        }
      }
    }

    // Keep inside the canvas
    for (let i = 0; i < count; i++) {
      points[i].x = Math.min(VIEW_WIDTH - PAD_X, Math.max(PAD_X, points[i].x));
      points[i].y = Math.min(VIEW_HEIGHT - PAD_Y, Math.max(PAD_Y, points[i].y));
    }
  }

  return points;
}

export default function KnowledgeGraphPage() {
  const { assets } = useData();
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const [session, setSession] = useState<PlantBrainSession | null>(null);

  const [graphNodes, setGraphNodes] = useState<ApiGraphNode[]>([]);
  const [graphEdges, setGraphEdges] = useState<ApiGraphEdge[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState(false);

  useEffect(() => {
    setSession(getStoredSession());
  }, []);

  useEffect(() => {
    let cancelled = false;

    const loadGraph = async () => {
      setLoading(true);
      setLoadError(false);
      try {
        const res = await apiFetch(appendPlantQuery('/api/graph'));
        if (!res.ok) {
          if (!cancelled) setLoadError(true);
          return;
        }
        const data = await res.json();
        if (cancelled) return;
        setGraphNodes(Array.isArray(data.nodes) ? data.nodes : []);
        setGraphEdges(Array.isArray(data.edges) ? data.edges : []);
      } catch {
        if (!cancelled) setLoadError(true);
      } finally {
        if (!cancelled) setLoading(false);
      }
    };

    loadGraph();

    return () => {
      cancelled = true;
    };
  }, []);

  const canViewCompliance = canRunAction(session?.role, 'view_compliance');

  // Force-directed placement with hard collision separation (no overlapping nodes).
  const { nodes, typeCount } = useMemo(() => {
    // Stable colour per entity type
    const types = Array.from(new Set(graphNodes.map(n => n.type || 'UNKNOWN')));
    const colorByType = new Map(types.map((type, i) => [type, i % TYPE_PALETTE.length]));

    // Degree drives node size, so well-connected entities read as more important
    const degree = new Map<string, number>();
    for (const edge of graphEdges) {
      degree.set(edge.source, (degree.get(edge.source) || 0) + 1);
      degree.set(edge.target, (degree.get(edge.target) || 0) + 1);
    }

    const points = computeLayout(graphNodes, graphEdges);
    const positioned: PositionedNode[] = graphNodes.map((node, i) => ({
      ...node,
      x: points[i].x,
      y: points[i].y,
      colorIndex: colorByType.get(node.type || 'UNKNOWN') ?? 0,
      degree: degree.get(node.id) || 0,
    }));

    return { nodes: positioned, typeCount: types.length };
  }, [graphNodes, graphEdges]);

  const nodeById = useMemo(() => new Map(nodes.map(node => [node.id, node])), [nodes]);

  // Only keep edges whose endpoints are both present in the returned node set.
  const edges = useMemo(
    () => graphEdges.filter(edge => nodeById.has(edge.source) && nodeById.has(edge.target)),
    [graphEdges, nodeById],
  );

  const distinctTypes = useMemo(() => {
    const seen = new Map<string, number>();
    nodes.forEach(node => {
      if (!seen.has(node.type)) seen.set(node.type, node.colorIndex);
    });
    return Array.from(seen.entries());
  }, [nodes]);

  const selectedNode = selectedNodeId ? nodeById.get(selectedNodeId) : null;
  const selectedEdgeCount = selectedNode
    ? edges.filter(edge => edge.source === selectedNode.id || edge.target === selectedNode.id).length
    : 0;

  const hasGraph = nodes.length > 0;
  const isAssetNode = (node: PositionedNode) => node.type.toLowerCase().includes('asset');

  // Neighbours of the selected node, used to dim everything unrelated
  const neighbourIds = useMemo(() => {
    if (!selectedNodeId) return null;
    const ids = new Set<string>([selectedNodeId]);
    for (const edge of edges) {
      if (edge.source === selectedNodeId) ids.add(edge.target);
      if (edge.target === selectedNodeId) ids.add(edge.source);
    }
    return ids;
  }, [selectedNodeId, edges]);

  const nodeRadius = (node: PositionedNode) => Math.min(26, 14 + node.degree * 2);

  return (
    <NavigationShell>
      <div className="space-y-6">
        <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
          <Metric label="Graph Entities" value={graphNodes.length} icon={<Network className="w-5 h-5" />} />
          <Metric label="Relationships" value={edges.length} icon={<Share2 className="w-5 h-5" />} />
          <Metric label="Entity Types" value={typeCount} icon={<FileText className="w-5 h-5" />} />
          <Metric label="High Risk Assets" value={assets.filter(a => a.riskScore >= 70).length} icon={<AlertTriangle className="w-5 h-5" />} />
        </div>

        <div className="glass-panel rounded-2xl border-slate-800/80 overflow-hidden">
          <div className="p-5 border-b border-slate-800/80 flex flex-col md:flex-row md:items-center justify-between gap-4">
            <div>
              <h3 className="text-base font-bold text-white tracking-wide">Asset Knowledge Graph</h3>
              <p className="text-xs text-slate-400 mt-0.5">Extracted entities and their relationships, sourced from the knowledge graph</p>
            </div>

            {distinctTypes.length > 0 && (
              <div className="flex flex-wrap gap-2 text-[10px] font-mono">
                {distinctTypes.map(([type, colorIndex]) => (
                  <span key={type} className="flex items-center gap-1.5 px-2 py-1 rounded-lg bg-slate-900/70 border border-slate-800 text-slate-300 uppercase tracking-wider">
                    <span className="w-2.5 h-2.5 rounded" style={{ backgroundColor: TYPE_PALETTE[colorIndex].stroke }} />
                    {type}
                  </span>
                ))}
              </div>
            )}
          </div>

          {hasGraph ? (
            <div className="overflow-x-auto">
              <svg
                viewBox={`0 0 ${VIEW_WIDTH} ${VIEW_HEIGHT}`}
                className="min-w-[900px] w-full bg-[#060918]"
                onClick={() => setSelectedNodeId(null)}
              >
                <defs>
                  <linearGradient id="edgeGradient" x1="0" x2="1">
                    <stop offset="0%" stopColor="#10b981" stopOpacity="0.6" />
                    <stop offset="100%" stopColor="#38bdf8" stopOpacity="0.3" />
                  </linearGradient>
                </defs>

                {/* Edges: gentle arcs so parallel links stay distinguishable */}
                {edges.map((edge, index) => {
                  const from = nodeById.get(edge.source);
                  const to = nodeById.get(edge.target);
                  if (!from || !to) return null;

                  const isRiskEdge = /gap|fail|risk|non.?compliance/i.test(edge.type);
                  const related = !neighbourIds || (neighbourIds.has(edge.source) && neighbourIds.has(edge.target));
                  const isIncident = Boolean(
                    selectedNodeId && (edge.source === selectedNodeId || edge.target === selectedNodeId),
                  );

                  // Curve perpendicular to the link for readability
                  const mx = (from.x + to.x) / 2;
                  const my = (from.y + to.y) / 2;
                  const dx = to.x - from.x;
                  const dy = to.y - from.y;
                  const len = Math.max(Math.hypot(dx, dy), 0.01);
                  const bow = 0.12;
                  const cx = mx - (dy / len) * len * bow;
                  const cy = my + (dx / len) * len * bow;

                  return (
                    <path
                      key={edge.id || `${edge.source}-${edge.target}-${index}`}
                      d={`M ${from.x} ${from.y} Q ${cx} ${cy} ${to.x} ${to.y}`}
                      fill="none"
                      stroke={isRiskEdge ? '#f43f5e' : 'url(#edgeGradient)'}
                      strokeOpacity={related ? (isIncident ? 0.9 : 0.4) : 0.06}
                      strokeWidth={isIncident ? 2.2 : 1.3}
                    />
                  );
                })}

                {/* Nodes: circle sized by connectivity, label beneath */}
                {nodes.map(node => {
                  const palette = TYPE_PALETTE[node.colorIndex];
                  const isSelected = selectedNodeId === node.id;
                  const related = !neighbourIds || neighbourIds.has(node.id);
                  const r = nodeRadius(node);

                  return (
                    <g
                      key={node.id}
                      onClick={event => {
                        event.stopPropagation();
                        setSelectedNodeId(isSelected ? null : node.id);
                      }}
                      className="cursor-pointer"
                      opacity={related ? 1 : 0.18}
                    >
                      <title>{`${node.label} — ${node.type}${node.degree ? ` (${node.degree} links)` : ''}`}</title>

                      {isSelected && (
                        <circle cx={node.x} cy={node.y} r={r + 7} fill="none" stroke={palette.stroke} strokeOpacity="0.35" strokeWidth="2" />
                      )}
                      <circle
                        cx={node.x}
                        cy={node.y}
                        r={r}
                        fill={palette.fill}
                        stroke={palette.stroke}
                        strokeOpacity={isSelected ? 1 : 0.75}
                        strokeWidth={isSelected ? 3 : 1.6}
                      />
                      <text
                        x={node.x}
                        y={node.y + r + 15}
                        textAnchor="middle"
                        fill={isSelected ? '#f8fafc' : '#cbd5e1'}
                        fontSize="11"
                        fontWeight={isSelected ? 700 : 600}
                        style={{ paintOrder: 'stroke', stroke: '#060918', strokeWidth: 3 } as React.CSSProperties}
                      >
                        {truncate(node.label, 18)}
                      </text>
                      <text
                        x={node.x}
                        y={node.y + r + 27}
                        textAnchor="middle"
                        fill="#64748b"
                        fontSize="8.5"
                        style={{ paintOrder: 'stroke', stroke: '#060918', strokeWidth: 3 } as React.CSSProperties}
                      >
                        {truncate(node.type, 20)}
                      </text>
                    </g>
                  );
                })}
              </svg>
            </div>
          ) : (
            <div className="py-20 flex flex-col items-center justify-center text-center gap-3 bg-[#060918]">
              {loading ? (
                <>
                  <Activity className="w-8 h-8 text-cyber-emerald animate-spin" />
                  <p className="text-xs text-slate-400 font-mono">Loading knowledge graph...</p>
                </>
              ) : (
                <>
                  <Network className="w-8 h-8 text-slate-600" />
                  <p className="text-sm font-bold text-white">No graph relationships yet</p>
                  <p className="text-xs text-slate-500 font-mono max-w-md">
                    {loadError
                      ? 'The knowledge graph could not be loaded. Try again once the API is reachable.'
                      : 'Entities and relationships appear here after documents are processed and linked.'}
                  </p>
                </>
              )}
            </div>
          )}

          {selectedNode && (
            <div className="border-t border-slate-800/80 p-5 flex flex-col md:flex-row md:items-center justify-between gap-4 bg-[#090d1f]/60">
              <div>
                <span className="text-[10px] text-slate-500 font-mono uppercase tracking-wider">{selectedNode.type} node</span>
                <h4 className="text-sm font-bold text-white mt-1">{selectedNode.label}</h4>
                <p className="text-xs text-slate-400 mt-1">
                  {(selectedNode.normalizedValue || selectedNode.type)} / {selectedEdgeCount} graph relationships visible
                </p>
              </div>
              {isAssetNode(selectedNode) && (
                <Link
                  href={`/assets/${encodeURIComponent(selectedNode.normalizedValue || selectedNode.label)}`}
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
            .filter(asset => asset.riskScore >= 60 || (canViewCompliance && asset.complianceGaps.length > 0))
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
