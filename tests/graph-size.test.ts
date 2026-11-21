import { describe, expect, it } from 'vitest';
import { createNode, DEFAULT_PARAMS, reconcileGraph, resolveGraphInputs, resolveInput, validateGraph } from '../src/domain/graph';
import { referenceOutputSubtotal } from '../src/domain/pricing';
import type { AssetVersion, Graph, InputSnapshot, Project } from '../src/domain/types';

const projectId = 'size-project';
function project(graph: Graph): Project { return { id: projectId, name: 'Dimensions', schemaVersion: 1, graphRevision: 1, layoutRevision: 1, createdAt: '', updatedAt: '', graph }; }
const asset: AssetVersion = { id: 'original', projectId, assetId: 'image', sha256: 'a'.repeat(64), mime: 'image/png', width: 64, height: 64, byteSize: 1000, createdAt: '', name: 'Original', parentVersionIds: [], source: 'import' };

describe('custom dimensions in graph snapshots', () => {
  it('freezes exact custom pixels for native, manual and mock runs', () => {
    const node = createNode('generate', projectId); node.config.text = 'A wide scene';
    node.config.params = { size: '1919x1080', quality: 'high', n: 1 };
    const p = project({ nodes: [node], edges: [] });
    const snapshots: InputSnapshot[] = [];
    for (const mode of ['codex', 'manual', 'mock'] as const) {
      const input = resolveInput(p, node.id, [], mode);
      expect(input.params.size).toBe('1919x1080');
      expect(resolveGraphInputs(p, [], mode)[node.id]).toEqual({ snapshot: input });
      snapshots.push(input);
    }
    node.config.params.size = '1280x720';
    expect(snapshots.every(snapshot => snapshot.params.size === '1919x1080')).toBe(true);
  });

  it('keeps a direct image edit source immutable with custom output dimensions', () => {
    const node = createNode('image', projectId); node.config.text = 'Create a portrait variation';
    node.selectedAssetVersionId = asset.id; node.config.params!.size = '1080x1920';
    const input = resolveInput(project({ nodes: [node], edges: [] }), node.id, [asset], 'codex');
    expect(input).toMatchObject({ nodeKind: 'edit', sourceAssetVersionId: asset.id, sourceAssetHash: asset.sha256, params: { size: '1080x1920' } });
    expect([asset.width, asset.height]).toEqual([64, 64]);
  });

  it('keeps connected parameters ahead of inline dimensions without changing either value', () => {
    const node = createNode('generate', projectId); node.config.text = 'A film frame';
    const params = createNode('params', projectId); params.config.params!.size = '1792x768';
    const graph: Graph = { nodes: [node, params], edges: [{ id: 'parameters', sourceNodeId: params.id, sourcePort: 'out', targetNodeId: node.id, targetPort: 'params' }] };
    expect(resolveInput(project(graph), node.id, [], 'manual').params.size).toBe('1792x768');
    expect(node.config.params!.size).toBe('1024x1024');
    expect(params.config.params!.size).toBe('1792x768');
  });

  it('rejects malformed serialized custom sizes at full-graph validation and resolution', () => {
    const node = createNode('generate', projectId); node.config.text = 'Draft';
    const p = project({ nodes: [node], edges: [] });
    for (const value of ['8192x8192', '0x64', '1920.5x1080', '99999x64', '1920X1080', NaN, null]) {
      Object.assign(node.config.params!, { size: value });
      expect(validateGraph(p.graph).valid).toBe(false);
      expect(() => resolveInput(p, node.id, [], 'mock')).toThrow('图片');
      expect(resolveGraphInputs(p, [], 'mock')[node.id].error).toContain('图片');
    }
  });

  it('retains the single-image native limit for expanded sizes', () => {
    const node = createNode('generate', projectId); node.config.text = 'A wide scene';
    node.config.params = { size: '1920x1080', quality: 'medium', n: 2 };
    const p = project({ nodes: [node], edges: [] });
    expect(() => resolveInput(p, node.id, [], 'codex')).toThrow('1 张');
    expect(resolveGraphInputs(p, [], 'codex')[node.id].error).toContain('1 张');
    expect(resolveInput(p, node.id, [], 'manual').params.n).toBe(2);
    expect(resolveInput(p, node.id, [], 'mock').params.n).toBe(2);
  });

  it('changes configuration version and stale state when custom pixels change', () => {
    const node = createNode('generate', projectId); node.config.text = 'Initial'; node.config.params!.size = '1920x1080'; node.selectedAssetVersionId = 'old-result';
    const next = createNode('edit', projectId); next.config.text = 'Next';
    const graph: Graph = { nodes: [node, next], edges: [{ id: 'image', sourceNodeId: node.id, sourcePort: 'out', targetNodeId: next.id, targetPort: 'image' }] };
    const incoming = structuredClone(graph); incoming.nodes[0].config.params!.size = '1919x1080';
    const saved = reconcileGraph(graph, incoming);
    expect(saved.nodes[0]).toMatchObject({ configVersion: 2, selectedAssetVersionId: 'old-result', stale: true });
    expect(saved.nodes[1].stale).toBe(true);
    expect(node.config.params!.size).toBe('1920x1080');
  });

  it.each(['1024x1024', '1536x1024', '1024x1536'] as const)('preserves legacy %s snapshot bytes and default values', size => {
    const node = createNode('generate', projectId); node.id = 'legacy-node'; node.config.text = 'Legacy'; node.config.params!.size = size;
    const input = resolveInput(project({ nodes: [node], edges: [] }), node.id, [], 'mock', 'mock-image-v1');
    const previous: InputSnapshot = { nodeId: 'legacy-node', nodeKind: 'generate', configVersion: 1, prompt: 'Legacy', params: { size, quality: 'medium', n: 1 }, upstreamStale: [], model: 'mock-image-v1', provider: 'mock' };
    expect(JSON.stringify(input)).toBe(JSON.stringify(previous));
    expect(DEFAULT_PARAMS).toEqual({ size: '1024x1024', quality: 'medium', n: 1 });
  });

  it('does not invent API reference prices for expanded dimensions', () => {
    const node = createNode('generate', projectId); node.config.text = 'Custom'; node.config.params!.size = '1920x1080';
    const input = resolveInput(project({ nodes: [node], edges: [] }), node.id, [], 'manual');
    const api = { ...input, provider: 'openai', model: 'gpt-image-1.5' };
    expect(referenceOutputSubtotal(api)).toBeUndefined();
    expect(referenceOutputSubtotal({ ...api, params: { ...api.params, size: '1024x1024' } })).toBe(0.034);
  });
});
