import type { CanvasEdge, CanvasNode, Graph, Project } from '../domain/types';
import { PORTS, validateConnection, validateGraph } from '../domain/graph';

export type ProjectDraft = Pick<Project, 'name' | 'graph'>;
export type MergeChoice = 'base' | 'local' | 'remote';
export type MergeResolutions = Readonly<Record<string, MergeChoice>>;
export interface MergeConflict {
  /** JSON Pointer; node/edge collection segments address IDs, never indexes. */
  path: string;
  kind: 'value' | 'delete_edit' | 'dangling_edge' | 'input_edges' | 'graph';
  base: unknown;
  local: unknown;
  remote: unknown;
  label: string;
  nodeId?: string;
  nodeName?: string;
  edgeId?: string;
  relatedNodeIds?: string[];
  relatedEdgeIds?: string[];
}
export interface ProjectMergeResult { merged: ProjectDraft; conflicts: MergeConflict[]; }
type ConflictLabel = Pick<MergeConflict, 'label' | 'nodeId' | 'nodeName' | 'edgeId'>;
const copy = <T>(value: T): T => structuredClone(value);
const segment = (value: string) => value.replaceAll('~', '~0').replaceAll('/', '~1');
const pointer = (parent: string, key: string) => `${parent}/${segment(key)}`;
const object = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value);
function canonical(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map(canonical).join(',')}]`;
  if (object(value)) return `{${Object.keys(value).filter(key => value[key] !== undefined).sort().map(key => `${JSON.stringify(key)}:${canonical(value[key])}`).join(',')}}`;
  return JSON.stringify(value) ?? 'undefined';
}
const equal = (a: unknown, b: unknown) => canonical(a) === canonical(b);
function semanticNode(node: CanvasNode | undefined): Record<string, unknown> | undefined {
  if (!node) return undefined;
  const { configVersion: _version, stale: _stale, ...value } = node;
  return value;
}
function index<T extends { id: string }>(values: T[]): Map<string, T> {
  const result = new Map<string, T>();
  for (const value of values) {
    if (result.has(value.id)) throw new Error(`重复的图元素 ID：${value.id}`);
    result.set(value.id, value);
  }
  return result;
}
function order<T extends { id: string }>(base: T[], local: T[], remote: T[]): string[] {
  return [...new Set([...remote, ...local, ...base].map(value => value.id))];
}
const fieldNames: Record<string, string> = { name: '名称', text: '提示词', params: '生成参数', size: '尺寸', quality: '质量', n: '张数', note: '备注', position: '位置', kind: '节点类型', selectedAssetVersionId: '选定图片版本' };

/**
 * Merge persisted draft data only. Viewport is tab-local; derived node metadata
 * comes from remote. The caller reconciles the returned graph against remote.
 * Unresolved field conflicts prefer local. Structural conflicts preserve both
 * proposals and select a valid local topology, never a dangling/cyclic graph.
 */
export function mergeProjectDraft(base: ProjectDraft, local: ProjectDraft, remote: ProjectDraft): ProjectMergeResult {
  return resolveProjectDraftConflicts(base, local, remote, {});
}

/** Recompute from the three originals so structural alternatives are not lost. */
export function resolveProjectDraftConflicts(base: ProjectDraft, local: ProjectDraft, remote: ProjectDraft, resolutions: MergeResolutions): ProjectMergeResult {
  const drafts = [base, local, remote];
  for (const draft of drafts) {
    const check = validateGraph({ nodes: draft.graph.nodes, edges: draft.graph.edges });
    if (!check.valid) throw new Error(`合并输入画布无效：${check.errors.join('；')}`);
  }
  const conflicts = new Map<string, MergeConflict>();
  const choice = (path: string): MergeChoice | undefined => {
    if (!Object.hasOwn(resolutions, path)) return undefined;
    const value = resolutions[path];
    if (!['base', 'local', 'remote'].includes(value)) throw new Error(`无效的冲突选择：${path}`);
    return value;
  };
  function conflict(path: string, kind: MergeConflict['kind'], b: unknown, l: unknown, r: unknown, label: ConflictLabel, related?: Pick<MergeConflict, 'relatedNodeIds' | 'relatedEdgeIds'>): unknown {
    const selected = choice(path);
    if (!selected) {
      const existing = conflicts.get(path);
      conflicts.set(path, existing ? { ...existing, ...related } : { path, kind, base: copy(b), local: copy(l), remote: copy(r), ...label, ...related });
    }
    return copy(selected === 'base' ? b : selected === 'remote' ? r : l);
  }
  function merge(b: unknown, l: unknown, r: unknown, path: string, label: ConflictLabel, atomic = false): unknown {
    if (equal(l, r)) return copy(l);
    if (equal(l, b)) return copy(r);
    if (equal(r, b)) return copy(l);
    if (!atomic && object(l) && object(r) && (b === undefined || object(b))) {
      const before = object(b) ? b : {};
      const keys = [...new Set([...Object.keys(before), ...Object.keys(l), ...Object.keys(r)])].sort();
      const entries: [string, unknown][] = [];
      for (const key of keys) {
        const value = merge(before[key], l[key], r[key], pointer(path, key), { ...label, label: `${label.nodeName ?? label.label} · ${fieldNames[key] ?? key}` }, key === 'position');
        if (value !== undefined) entries.push([key, value]);
      }
      return Object.fromEntries(entries);
    }
    return conflict(path, b !== undefined && (l === undefined || r === undefined) ? 'delete_edit' : 'value', b, l, r, label);
  }
  const nodeMaps = drafts.map(draft => index(draft.graph.nodes));
  const edgeMaps = drafts.map(draft => index(draft.graph.edges));
  const nodeOrder = order(base.graph.nodes, local.graph.nodes, remote.graph.nodes);
  const edgeOrder = order(base.graph.edges, local.graph.edges, remote.graph.edges);
  const nodeLabel = (id: string): ConflictLabel => {
    const node = nodeMaps[1].get(id) ?? nodeMaps[2].get(id) ?? nodeMaps[0].get(id);
    return { label: `节点“${node?.name ?? id}”`, nodeId: id, nodeName: node?.name ?? id };
  };
  const withMetadata = (node: Record<string, unknown>): CanvasNode => {
    const current = nodeMaps[2].get(node.id as string);
    return { ...copy(node), configVersion: current?.configVersion ?? 1, stale: current?.stale ?? false } as unknown as CanvasNode;
  };
  let nodes = new Map<string, CanvasNode>();
  for (const id of nodeOrder) {
    const value = merge(...nodeMaps.map(map => semanticNode(map.get(id))) as [unknown, unknown, unknown], pointer('/graph/nodes', id), nodeLabel(id));
    if (value !== undefined) nodes.set(id, withMetadata(value as Record<string, unknown>));
  }
  let edges = new Map<string, CanvasEdge>();
  for (const id of edgeOrder) {
    // A connection is an atomic endpoint tuple: mixing two retarget operations
    // could produce a connection that neither author made and that is invalid.
    const value = merge(...edgeMaps.map(map => map.get(id)) as [unknown, unknown, unknown], pointer('/graph/edges', id), { label: '连线', edgeId: id }, true);
    if (value !== undefined) edges.set(id, value as CanvasEdge);
  }
  function repairReferences() {
    const missing = new Map<string, string[]>();
    for (const edge of edges.values()) for (const id of [edge.sourceNodeId, edge.targetNodeId]) {
      if (!nodes.has(id)) missing.set(id, [...new Set([...(missing.get(id) ?? []), edge.id])]);
    }
    for (const [id, edgeIds] of missing) {
      const selected = conflict(pointer('/graph/nodes', id), 'dangling_edge', ...nodeMaps.map(map => semanticNode(map.get(id))) as [unknown, unknown, unknown], { ...nodeLabel(id), label: `${nodeLabel(id).label} · 删除与关联连线冲突` }, { relatedNodeIds: [id], relatedEdgeIds: edgeIds });
      if (selected !== undefined) nodes.set(id, withMetadata(selected as Record<string, unknown>));
      else for (const [edgeId, edge] of edges) if (edge.sourceNodeId === id || edge.targetNodeId === id) edges.delete(edgeId);
    }
  }
  repairReferences();
  const groups = new Map<string, CanvasEdge[]>();
  for (const edge of edges.values()) {
    const key = JSON.stringify([edge.targetNodeId, edge.targetPort]);
    groups.set(key, [...(groups.get(key) ?? []), edge]);
  }
  for (const group of groups.values()) {
    if (group.length < 2) continue;
    const { targetNodeId, targetPort } = group[0];
    const target = nodes.get(targetNodeId);
    if (target && PORTS[target.kind].inputs.find(port=>port.id===targetPort)?.multiple && group.length <= 8) continue;
    const alternatives = drafts.map(draft => draft.graph.edges.filter(edge => edge.targetNodeId === targetNodeId && edge.targetPort === targetPort));
    const selected = conflict(pointer(pointer('/graph/inputs', targetNodeId), targetPort), 'input_edges', ...alternatives as [unknown, unknown, unknown], { ...nodeLabel(targetNodeId), label: `${nodeLabel(targetNodeId).label} · 输入 ${targetPort} 的并发连线` }, { relatedNodeIds: [targetNodeId], relatedEdgeIds: group.map(edge => edge.id) }) as CanvasEdge[];
    for (const edge of group) edges.delete(edge.id);
    for (const edge of selected) edges.set(edge.id, copy(edge));
  }
  repairReferences();
  // Two individually valid DAGs can jointly form a cycle. Resolve only each
  // cyclic component; independent additions in other components are retained.
  for (const ids of cyclicComponents([...nodes.keys()], [...edges.values()])) {
    const members = new Set(ids);
    const alternatives = drafts.map(draft => draft.graph.edges.filter(edge => members.has(edge.sourceNodeId) && members.has(edge.targetNodeId)));
    const selected = conflict(pointer('/graph/topology', JSON.stringify([...ids].sort())), 'graph', ...alternatives as [unknown, unknown, unknown], { label: '并发连线形成环路' }, { relatedNodeIds: ids, relatedEdgeIds: [...edges.values()].filter(edge => members.has(edge.sourceNodeId) && members.has(edge.targetNodeId)).map(edge => edge.id) }) as CanvasEdge[];
    for (const [id, edge] of edges) if (members.has(edge.sourceNodeId) && members.has(edge.targetNodeId)) edges.delete(id);
    for (const edge of selected) edges.set(edge.id, copy(edge));
  }
  // A node-kind change and a new connection can each be valid in isolation,
  // yet disagree about the available ports. Choose topology only within the
  // affected connected component, retaining merged names/config/layout.
  const currentEdges = [...edges.values()];
  const currentNodes = [...nodes.values()];
  const invalidIds = new Set(currentEdges.filter(edge => !validateConnection({ nodes: currentNodes, edges: currentEdges.filter(other => other.id !== edge.id) }, edge).valid).flatMap(edge => [edge.sourceNodeId, edge.targetNodeId]));
  if (invalidIds.size) {
    const allEdges = [...currentEdges, ...drafts.flatMap(draft => draft.graph.edges)];
    const pending = new Set(invalidIds);
    while (pending.size) {
      const ids = new Set([pending.values().next().value as string]);
      for (const id of ids) for (const edge of allEdges) {
        if (edge.sourceNodeId === id) ids.add(edge.targetNodeId);
        if (edge.targetNodeId === id) ids.add(edge.sourceNodeId);
      }
      for (const id of ids) pending.delete(id);
      const alternatives = drafts.map(draft => ({ nodes: draft.graph.nodes.filter(node => ids.has(node.id)), edges: draft.graph.edges.filter(edge => ids.has(edge.sourceNodeId) && ids.has(edge.targetNodeId)) }));
      const selected = conflict(pointer('/graph/ports', JSON.stringify([...ids].sort())), 'graph', ...alternatives as [unknown, unknown, unknown], { label: '节点类型与连接端口冲突' }, { relatedNodeIds: [...ids], relatedEdgeIds: currentEdges.filter(edge => ids.has(edge.sourceNodeId) && ids.has(edge.targetNodeId)).map(edge => edge.id) }) as Graph;
      for (const node of selected.nodes) {
        const merged = nodes.get(node.id);
        nodes.set(node.id, merged ? { ...merged, kind: node.kind } : withMetadata(semanticNode(node)!));
      }
      for (const [id, edge] of edges) if (ids.has(edge.sourceNodeId) && ids.has(edge.targetNodeId)) edges.delete(id);
      for (const edge of selected.edges) edges.set(edge.id, copy(edge));
    }
    repairReferences();
  }
  const name = merge(base.name, local.name, remote.name, '/name', { label: '项目名称' }) as string;
  let graph: Graph = { nodes: nodeOrder.filter(id => nodes.has(id)).map(id => nodes.get(id)!), edges: edgeOrder.filter(id => edges.has(id)).map(id => edges.get(id)!) };
  if (!validateGraph(graph).valid) {
    // Concurrent node-kind/connection edits can also invalidate ports. Keep the
    // complete alternatives explicit instead of returning an unsaveable graph.
    const alternatives = drafts.map(draft => ({ nodes: draft.graph.nodes, edges: draft.graph.edges }));
    const selected = conflict('/graph/structure', 'graph', ...alternatives as [unknown, unknown, unknown], { label: '节点类型与连线结构冲突' }) as Graph;
    graph = { nodes: selected.nodes.map(node => withMetadata(semanticNode(node)!)), edges: copy(selected.edges) };
  }
  return { merged: { name, graph }, conflicts: [...conflicts.values()] };
}

function cyclicComponents(ids: string[], edges: CanvasEdge[]): string[][] {
  const adjacency = new Map(ids.map(id => [id, [] as string[]]));
  for (const edge of edges) adjacency.get(edge.sourceNodeId)?.push(edge.targetNodeId);
  const indexes = new Map<string, number>(), low = new Map<string, number>(), active = new Set<string>(), stack: string[] = [], result: string[][] = [];
  let sequence = 0;
  function visit(id: string) {
    indexes.set(id, sequence); low.set(id, sequence++); stack.push(id); active.add(id);
    for (const target of adjacency.get(id) ?? []) {
      if (!indexes.has(target)) { visit(target); low.set(id, Math.min(low.get(id)!, low.get(target)!)); }
      else if (active.has(target)) low.set(id, Math.min(low.get(id)!, indexes.get(target)!));
    }
    if (low.get(id) !== indexes.get(id)) return;
    const component: string[] = [];let next: string;
    do { next = stack.pop()!; active.delete(next); component.push(next); } while (next !== id);
    if (component.length > 1 || (adjacency.get(id) ?? []).includes(id)) result.push(component);
  }
  for (const id of ids) if (!indexes.has(id)) visit(id);
  return result;
}
