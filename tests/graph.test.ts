import { describe, it, expect } from 'vitest';
import { createNode, createImageFirstGraph, createStarterGraph, DEFAULT_PARAMS, descendants, getNodeOutputAsset, reconcileGraph, resolveGraphInputs, resolveInput, validateConnection, validateGraph } from '../src/domain/graph';
import type { AssetVersion, CanvasEdge, Graph, Project } from '../src/domain/types';

const projectId = 'project-test';
function edge(sourceNodeId: string, targetNodeId: string, targetPort: string): CanvasEdge { return { id: `${sourceNodeId}-${targetNodeId}-${targetPort}`, sourceNodeId, sourcePort: 'out', targetNodeId, targetPort }; }
function project(graph: Graph): Project { return { id: projectId, name: 'test', schemaVersion: 1, graphRevision: 1, layoutRevision: 1, createdAt: '', updatedAt: '', graph }; }
function asset(id = 'asset-1', otherProject = projectId): AssetVersion { return { id, projectId: otherProject, assetId: 'a', sha256: 'a'.repeat(64), mime: 'image/png', width: 64, height: 64, byteSize: 1000, createdAt: '', name: 'test', parentVersionIds: [], source: 'import' }; }
function chain() {
  const text = createNode('text', projectId); text.config.text = 'Keep the viewpoint';
  const image = createNode('image', projectId); image.selectedAssetVersionId = 'asset-1';
  const edit = createNode('edit', projectId); edit.selectedAssetVersionId = 'asset-2';
  const next = createNode('edit', projectId); next.selectedAssetVersionId = 'asset-3';
  const out = createNode('output', projectId);
  const graph: Graph = { nodes: [text, image, edit, next, out], edges: [edge(text.id, edit.id, 'prompt'), edge(image.id, edit.id, 'image'), edge(text.id, next.id, 'prompt'), edge(edit.id, next.id, 'image'), edge(next.id, out.id, 'image')] };
  return { graph, text, image, edit, next, out, assets: [asset(), asset('asset-2'), asset('asset-3')] };
}

describe('typed DAG', () => {
  it('accepts six-node starter graph without invoking any provider', () => { const graph = createStarterGraph(projectId); expect(graph.nodes).toHaveLength(6); expect(validateGraph(graph).valid).toBe(true); });
  it('rejects wrong types, self-loop, duplicate, occupied port and reversed direction', () => {
    const { graph, text, image, edit, next } = chain();
    expect(validateConnection(graph, edge(image.id, edit.id, 'prompt')).errors.join()).toContain('不兼容');
    expect(validateConnection(graph, edge(edit.id, edit.id, 'image')).errors.join()).toContain('自身');
    expect(validateConnection(graph, edge(text.id, edit.id, 'prompt')).errors.join()).toContain('已经连接');
    expect(validateConnection(graph, edge(next.id, edit.id, 'image')).errors.join()).toContain('最多');
    expect(validateConnection(graph, { ...edge(text.id, edit.id, 'out'), sourcePort: 'prompt' }).valid).toBe(false);
  });
  it('rejects closed loops and missing nodes on complete save', () => {
    const { graph, edit, next } = chain();
    const cyclic = { ...graph, edges: [...graph.edges.filter(e => e.targetNodeId !== edit.id || e.targetPort !== 'image'), edge(next.id, edit.id, 'image')] };
    expect(validateGraph(cyclic).errors.join()).toContain('环路');
    expect(validateConnection(graph, edge('absent', edit.id, 'image')).errors.join()).toContain('不存在');
  });
  it('rejects duplicate node/edge IDs, invalid params and cross-project assets', () => {
    const { graph, text } = chain();
    expect(validateGraph({ ...graph, nodes: [...graph.nodes, text] }).valid).toBe(false);
    expect(validateGraph({ ...graph, edges: [...graph.edges, graph.edges[0]] }).valid).toBe(false);
    const params = createNode('params', projectId); params.config.params!.n = 5;
    expect(validateGraph({ ...graph, nodes: [...graph.nodes, params] }).valid).toBe(false);
    expect(validateGraph(graph, [asset('asset-1', 'other-project'), asset('asset-2'), asset('asset-3')]).errors.join()).toContain('其他项目');
  });
  it('terminates reachability even on malformed cycles', () => {
    const { graph, edit, next } = chain(); graph.edges.push(edge(next.id, edit.id, 'image'));
    expect(descendants(graph, edit.id)).toContain(next.id);
    expect(descendants(graph, edit.id)).not.toContain(edit.id);
  });
});

describe('configuration and invalidation', () => {
  it('propagates stale without deleting selected assets and ignores caller revision/stale claims', () => {
    const { graph, text, edit, next, out } = chain();
    const draft = structuredClone(graph); draft.nodes.find(n => n.id === text.id)!.config.text = 'Change the background';
    draft.nodes.forEach(n => { n.configVersion = 999; n.stale = false; });
    const saved = reconcileGraph(graph, draft);
    expect(saved.nodes.find(n => n.id === text.id)!.configVersion).toBe(2);
    for (const id of [edit.id, next.id, out.id]) expect(saved.nodes.find(n => n.id === id)!.stale).toBe(true);
    expect(saved.nodes.find(n => n.id === edit.id)!.selectedAssetVersionId).toBe('asset-2');
    expect(saved.nodes.find(n => n.id === next.id)!.configVersion).toBe(1);
  });
  it('position, zoom, rename and notes do not invalidate results or increment configVersion', () => {
    const { graph, text, assets, edit } = chain();
    const before = resolveInput(project(graph), edit.id, assets, 'mock');
    const draft = structuredClone(graph); const node = draft.nodes.find(n => n.id === text.id)!;
    node.name = 'renamed'; node.position.x += 100; node.config.note = 'a note'; draft.viewport = { x: 100, y: 20, zoom: 0.8 };
    const saved = reconcileGraph(graph, draft);
    expect(saved.nodes.every(n => !n.stale)).toBe(true);
    expect(resolveInput(project(saved), edit.id, assets, 'mock')).toEqual(before);
  });
  it('selecting a new output version invalidates descendants without making the source stale', () => {
    const { graph, edit, next } = chain();
    const draft = structuredClone(graph); draft.nodes.find(n => n.id === edit.id)!.selectedAssetVersionId = 'asset-new';
    const saved = reconcileGraph(graph, draft);
    expect(saved.nodes.find(n => n.id === edit.id)!.stale).toBe(false);
    expect(saved.nodes.find(n => n.id === next.id)!.stale).toBe(true);
    expect(graph.nodes.find(n => n.id === edit.id)!.selectedAssetVersionId).toBe('asset-2');
  });
  it('removing/replacing an input edge invalidates the target and descendants', () => {
    const { graph, edit, next, out } = chain();
    const draft = structuredClone(graph); draft.edges = draft.edges.filter(e => !(e.targetNodeId === edit.id && e.targetPort === 'image'));
    const saved = reconcileGraph(graph, draft);
    for (const id of [edit.id, next.id, out.id]) expect(saved.nodes.find(n => n.id === id)!.stale).toBe(true);
    expect(saved.nodes.find(n => n.id === edit.id)!.configVersion).toBe(2);
  });
  it('new nodes have a fresh version/state', () => {
    const original = createNode('edit', projectId); const incoming = structuredClone(original); incoming.configVersion = 42; incoming.stale = true;
    expect(reconcileGraph({ nodes: [], edges: [] }, { nodes: [incoming], edges: [] }).nodes[0]).toMatchObject({ configVersion: 1, stale: false });
  });
});

describe('snapshot resolution', () => {
  it('requires connected prompt and an explicitly selected input image', () => {
    const { graph, text, edit, assets, image } = chain();
    text.config.text = '';
    expect(() => resolveInput(project(graph), edit.id, assets, 'mock')).toThrow('非空');
    text.config.text = 'prompt'; image.selectedAssetVersionId = undefined;
    expect(() => resolveInput(project(graph), edit.id, assets, 'mock')).toThrow('选定');
  });
  it('freezes resolved values and returns selected input version/hash and stale ancestors', () => {
    const { graph, text, edit, next, assets, out } = chain(); edit.stale = true;
    const input = resolveInput(project(graph), next.id, assets, 'mock');
    expect(input).toMatchObject({ prompt: text.config.text, sourceAssetVersionId: 'asset-2', sourceAssetHash: 'a'.repeat(64), upstreamStale: [edit.id], provider: 'mock' });
    text.config.text = 'later'; next.config.params!.n = 4;
    expect(input.prompt).toBe('Keep the viewpoint'); expect(input.params.n).toBe(1);
    expect(getNodeOutputAsset(graph, out.id)).toBe('asset-3');
  });
  it('uses a connected params node before local node parameters', () => {
    const { graph, edit, assets } = chain(); const params = createNode('params', projectId); params.config.params = { ...DEFAULT_PARAMS, n: 2 };
    graph.nodes.push(params); graph.edges.push(edge(params.id, edit.id, 'params'));
    expect(resolveInput(project(graph), edit.id, assets, 'manual').params.n).toBe(2);
  });
  it('rejects missing/foreign/corrupt assets and running non-executable nodes', () => {
    const { graph, edit, text, assets } = chain();
    expect(() => resolveInput(project(graph), edit.id, [], 'mock')).toThrow('不存在');
    expect(() => resolveInput(project(graph), edit.id, [asset('asset-1', 'other'), ...assets.slice(1)], 'mock')).toThrow('其他项目');
    expect(() => resolveInput(project(graph), edit.id, [{ ...assets[0], sha256: 'bad' }, ...assets.slice(1)], 'mock')).toThrow('校验');
    expect(() => resolveInput(project(graph), text.id, assets, 'mock')).toThrow('只有');
  });
});

describe('untrusted serialized graph shapes', () => {
  it('returns validation errors for malformed nodes, edges, configs and position without throwing', () => {
    const node = createNode('text', projectId);
    const cases: unknown[] = [null, {}, { nodes: null, edges: [] }, { nodes: [null], edges: [] }, { nodes: [node, null], edges: [edge(node.id, 'x', 'prompt')] }, { nodes: [{ ...node, config: null }], edges: [] }, { nodes: [{ ...node, position: null }], edges: [] }, { nodes: [node], edges: [null] }, { nodes: [node], edges: [{ id: 'e' }] }, { nodes: [node], edges: [], viewport: { x: 0, y: 0, zoom: 0 } }];
    for (const value of cases) {
      expect(() => validateGraph(value as Graph)).not.toThrow();
      expect(validateGraph(value as Graph).valid).toBe(false);
    }
  });
  it('does not allow reversed input direction or ImageRef copied onto a Text port', () => {
    const text = createNode('text', projectId); const image = createNode('image', projectId); const edit = createNode('edit', projectId);
    const graph = { nodes: [text, image, edit], edges: [] };
    expect(validateConnection(graph, { ...edge(edit.id, image.id, 'out'), sourcePort: 'image' }).valid).toBe(false);
    expect(validateConnection(graph, edge(image.id, edit.id, 'prompt')).valid).toBe(false);
    expect(validateConnection(graph, edge(text.id, edit.id, 'image')).valid).toBe(false);
  });
});


describe('batch input resolution', () => {
  it('matches single-node snapshots for every runnable node and execution mode', () => {
    const { graph, edit, next, assets, text, image, out } = chain(); edit.stale = true; image.config.text = 'Edit this selected image';
    for (const mode of ['mock', 'manual', 'api', 'codex'] as const) {
      const resolved = resolveGraphInputs(project(graph), assets, mode, 'explicit-model');
      expect(Object.keys(resolved).sort()).toEqual([image.id, edit.id, next.id].sort());
      for (const node of [image, edit, next]) expect(resolved[node.id]).toEqual({ snapshot: resolveInput(project(graph), node.id, assets, mode, 'explicit-model') });
      for (const node of [text, out]) expect(resolved[node.id]).toBeUndefined();
    }
  });
  it('returns individual missing-input errors while retaining valid runnable snapshots', () => {
    const text = createNode('text', projectId); text.config.text = 'Ready prompt';
    const ready = createNode('generate', projectId); const noPrompt = createNode('generate', projectId); const noImage = createNode('edit', projectId);
    const graph: Graph = { nodes: [text, ready, noPrompt, noImage], edges: [edge(text.id, ready.id, 'prompt'), edge(text.id, noImage.id, 'prompt')] };
    const resolved = resolveGraphInputs(project(graph), [], 'mock');
    expect(resolved[ready.id].snapshot).toEqual(resolveInput(project(graph), ready.id, [], 'mock'));
    for (const node of [noPrompt, noImage]) {
      let message = ''; try { resolveInput(project(graph), node.id, [], 'mock'); } catch (error) { message = (error as Error).message; }
      expect(resolved[node.id]).toEqual({ error: message });
    }
  });
  it('preserves full-graph and cross-project validation instead of silently skipping unrelated errors', () => {
    const { graph, edit, next, assets } = chain();
    graph.nodes.push(createNode('params', projectId)); graph.nodes.at(-1)!.config.params!.n = 9;
    const resolved = resolveGraphInputs(project(graph), assets, 'mock');
    expect(resolved[edit.id].error).toContain('1–4'); expect(resolved[next.id].error).toContain('1–4');
    expect(() => resolveInput(project(graph), edit.id, assets, 'mock')).toThrow('1–4');
    const foreign = chain();
    const other = resolveGraphInputs({ ...project(foreign.graph), id: 'other-project' }, foreign.assets, 'mock');
    expect(other[foreign.edit.id].error).toBe('节点不属于当前项目');
  });
  it('owns parameter snapshots and safely resolves prototype-like node IDs', () => {
    const text = createNode('text', projectId); text.config.text = 'Prompt';
    const node = createNode('generate', projectId); node.id = '__proto__';
    const graph: Graph = { nodes: [text, node], edges: [edge(text.id, node.id, 'prompt')] };
    const resolved = resolveGraphInputs(project(graph), [], 'mock');
    expect(Object.hasOwn(resolved, '__proto__')).toBe(true);
    expect(resolved[node.id].snapshot?.prompt).toBe('Prompt');
    node.config.params!.n = 4; text.config.text = 'Changed';
    expect(resolved[node.id].snapshot?.params.n).toBe(1);
    expect(resolved[node.id].snapshot?.prompt).toBe('Prompt');
  });
});


describe('image-first inline inputs', () => {
  it('starts a new canvas with one empty image card and keeps the advanced template available', () => {
    const graph = createImageFirstGraph(projectId);
    expect(graph.nodes).toHaveLength(1);
    expect(graph.nodes[0]).toMatchObject({ kind: 'generate', config: { text: '', params: DEFAULT_PARAMS }, stale: false });
    expect(graph.edges).toEqual([]);
    expect(graph.nodes[0].selectedAssetVersionId).toBeUndefined();
    expect(validateGraph(graph).valid).toBe(true);
    expect(() => resolveInput(project(graph), graph.nodes[0].id, [], 'mock')).toThrow('侧栏');
    expect(createStarterGraph(projectId).nodes).toHaveLength(6);
  });

  it.each(['generate', 'edit'] as const)('%s resolves inline prompt and parameters without text or parameter cards', kind => {
    const node = createNode(kind, projectId); node.config.text = 'Keep the object, soften the light';
    node.config.params = { size: '1536x1024', quality: 'high', n: 2 };
    const image = createNode('image', projectId); image.selectedAssetVersionId = 'asset-1';
    const graph: Graph = { nodes: kind === 'edit' ? [node, image] : [node], edges: kind === 'edit' ? [edge(image.id, node.id, 'image')] : [] };
    const snapshot = resolveInput(project(graph), node.id, [asset()], 'mock');
    expect(snapshot).toMatchObject({ nodeKind: kind, prompt: node.config.text, params: node.config.params });
    expect(snapshot.sourceAssetVersionId).toBe(kind === 'edit' ? 'asset-1' : undefined);
    node.config.text = 'Later draft'; node.config.params.n = 4;
    expect(snapshot.prompt).toBe('Keep the object, soften the light');
    expect(snapshot.params.n).toBe(2);
  });

  it('keeps linked prompt ownership and never falls back from an empty connected prompt', () => {
    const node = createNode('generate', projectId); node.config.text = 'Inline alternative';
    const text = createNode('text', projectId); text.config.text = 'Connected prompt';
    const graph: Graph = { nodes: [node, text], edges: [edge(text.id, node.id, 'prompt')] };
    expect(resolveInput(project(graph), node.id, [], 'mock').prompt).toBe('Connected prompt');
    text.config.text = '   ';
    expect(() => resolveInput(project(graph), node.id, [], 'mock')).toThrow('连接');
    graph.edges = [];
    expect(resolveInput(project(graph), node.id, [], 'mock').prompt).toBe('Inline alternative');
    node.config.text = '  ';
    expect(() => resolveInput(project(graph), node.id, [], 'mock')).toThrow('非空');
  });

  it('maps a plain selected image to an edit snapshot while preserving its immutable base version', () => {
    const image = createNode('image', projectId); image.config.text = 'Change the background';
    image.config.params = { size: '1024x1536', quality: 'high', n: 1 };
    image.selectedAssetVersionId = 'asset-1';
    const graph: Graph = { nodes: [image], edges: [] };
    const assets = [asset(), { ...asset('asset-2'), parentVersionIds: ['asset-1'] }];
    for (const mode of ['mock', 'manual', 'api', 'codex'] as const) {
      const snapshot = resolveInput(project(graph), image.id, assets, mode);
      expect(snapshot).toMatchObject({ nodeId: image.id, nodeKind: 'edit', prompt: image.config.text, params: image.config.params, sourceAssetVersionId: 'asset-1', sourceAssetHash: assets[0].sha256 });
      expect(getNodeOutputAsset(graph, image.id)).toBe('asset-1');
      expect(resolveGraphInputs(project(graph), assets, mode)[image.id]).toEqual({ snapshot });
    }
    const frozen = resolveInput(project(graph), image.id, assets, 'mock');
    image.selectedAssetVersionId = 'asset-2'; image.config.text = 'Edit the adopted result'; image.config.params.n = 3;
    expect(frozen).toMatchObject({ sourceAssetVersionId: 'asset-1', prompt: 'Change the background', params: { n: 1 } });
    expect(resolveInput(project(graph), image.id, assets, 'mock').sourceAssetVersionId).toBe('asset-2');
    expect(assets[0].parentVersionIds).toEqual([]);
  });

  it('retains explicit image input requirements for edit and does not turn a generate output into its input', () => {
    const edit = createNode('edit', projectId); edit.config.text = 'An inline edit'; edit.selectedAssetVersionId = 'asset-1';
    expect(() => resolveInput(project({ nodes: [edit], edges: [] }), edit.id, [asset()], 'mock')).toThrow('连接一张主图');
    const generated = createNode('generate', projectId); generated.config.text = 'A new image'; generated.selectedAssetVersionId = 'asset-1';
    const snapshot = resolveInput(project({ nodes: [generated], edges: [] }), generated.id, [asset()], 'mock');
    expect(snapshot.nodeKind).toBe('generate');
    expect(snapshot.sourceAssetVersionId).toBeUndefined();
    const image = createNode('image', projectId); image.config.text = 'Edit independently'; image.selectedAssetVersionId = 'asset-1';
    const text = createNode('text', projectId); text.config.text = 'Advanced text';
    expect(validateConnection({ nodes: [image, text], edges: [] }, edge(text.id, image.id, 'prompt')).valid).toBe(false);
  });

  it('validates missing, foreign and not-ready self-selected image assets', () => {
    const image = createNode('image', projectId); image.config.text = 'Edit this image';
    const p = project({ nodes: [image], edges: [] });
    expect(() => resolveInput(p, image.id, [], 'mock')).toThrow('导入图片');
    image.selectedAssetVersionId = 'asset-1';
    expect(() => resolveInput(p, image.id, [], 'mock')).toThrow('不存在');
    expect(() => resolveInput(p, image.id, [asset('asset-1', 'foreign')], 'mock')).toThrow('其他项目');
    for (const notReady of [{ ...asset(), sha256: 'bad' }, { ...asset(), width: 0 }, { ...asset(), mime: 'image/gif' }, { ...asset(), byteSize: 21 * 1024 * 1024 }]) {
      expect(() => resolveInput(p, image.id, [notReady], 'mock')).toThrow('校验');
    }
    expect(resolveInput(p, image.id, [asset()], 'mock').sourceAssetVersionId).toBe('asset-1');
  });

  it.each(['image', 'generate', 'edit'] as const)('%s inline prompt and parameter changes version and invalidate dependent results', kind => {
    const node = createNode(kind, projectId); node.config.text = 'Initial prompt'; node.selectedAssetVersionId = 'asset-1';
    const next = createNode('edit', projectId); next.config.text = 'Next edit'; next.selectedAssetVersionId = 'asset-2';
    const output = createNode('output', projectId);
    const graph: Graph = { nodes: [node, next, output], edges: [edge(node.id, next.id, 'image'), edge(next.id, output.id, 'image')] };
    for (const change of ['text', 'params'] as const) {
      const incoming = structuredClone(graph); const draft = incoming.nodes[0];
      if (change === 'text') draft.config.text = 'Changed prompt'; else draft.config.params!.quality = 'high';
      const saved = reconcileGraph(graph, incoming);
      expect(saved.nodes[0].configVersion).toBe(2);
      expect(saved.nodes.every(item => item.stale)).toBe(true);
      expect(saved.nodes[0].selectedAssetVersionId).toBe('asset-1');
      expect(saved.nodes[1].selectedAssetVersionId).toBe('asset-2');
      expect(graph.nodes.every(item => !item.stale)).toBe(true);
    }
  });

  it('reports a stale image ancestor and invalidates descendants on explicit version selection', () => {
    const { graph, image, edit, next, assets } = chain(); image.config.text = 'Direct image draft'; image.stale = true;
    expect(resolveInput(project(graph), edit.id, assets, 'mock').upstreamStale).toContain(image.id);
    expect(resolveInput(project(graph), next.id, assets, 'mock').upstreamStale).toContain(image.id);
    const incoming = structuredClone(graph); incoming.nodes.find(node => node.id === image.id)!.selectedAssetVersionId = 'asset-new';
    const saved = reconcileGraph(graph, incoming);
    expect(saved.nodes.find(node => node.id === edit.id)!.stale).toBe(true);
    expect(saved.nodes.find(node => node.id === next.id)!.stale).toBe(true);
    expect(getNodeOutputAsset(graph, image.id)).toBe('asset-1');
  });

  it('owns default inline parameter objects and still validates the whole graph for image execution', () => {
    const image = createNode('image', projectId); image.config.text = 'Edit'; image.selectedAssetVersionId = 'asset-1';
    const generated = createNode('generate', projectId); generated.config.text = 'Generate';
    image.config.params!.n = 2;
    expect(generated.config.params!.n).toBe(1);
    expect(DEFAULT_PARAMS.n).toBe(1);
    const params = createNode('params', projectId); params.config.params!.n = 9;
    const p = project({ nodes: [image, generated, params], edges: [] });
    expect(() => resolveInput(p, image.id, [asset()], 'mock')).toThrow('1–4');
    const resolved = resolveGraphInputs(p, [asset()], 'mock');
    expect(resolved[image.id].error).toContain('1–4');
    expect(resolved[generated.id].error).toContain('1–4');
  });
});
