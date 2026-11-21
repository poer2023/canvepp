import { describe, it, expect } from 'vitest';
import { createNode, descendants, reconcileGraph, resolveGraphInputs, validateGraph } from '../src/domain/graph';
import type { CanvasEdge, Graph, Project } from '../src/domain/types';

function denseGraph(): Graph {
  const text = createNode('text', 'stress'); text.config.text = 'Stress graph prompt';
  const params = createNode('params', 'stress');
  const nodes = [text, params, ...Array.from({ length: 98 }, (_, index) => { const node = createNode('generate', 'stress', { x: (index % 10) * 330, y: Math.floor(index / 10) * 280 }); node.selectedAssetVersionId = `asset-${index}`; return node; })];
  const edges: CanvasEdge[] = nodes.slice(2).map(node => ({ id: `prompt-${node.id}`, sourceNodeId: text.id, sourcePort: 'out', targetNodeId: node.id, targetPort: 'prompt' }));
  for (const node of nodes.slice(2, 54)) edges.push({ id: `params-${node.id}`, sourceNodeId: params.id, sourcePort: 'out', targetNodeId: node.id, targetPort: 'params' });
  return { nodes, edges };
}

describe('100 node / 150 edge domain graph', () => {
  it('validates, preserves structure and marks all text descendants stale', () => {
    const graph = denseGraph();
    expect(graph.nodes).toHaveLength(100); expect(graph.edges).toHaveLength(150);
    const started = performance.now();
    expect(validateGraph(graph).valid).toBe(true);
    const draft = structuredClone(graph); draft.nodes[0].config.text = 'A changed prompt';
    const saved = reconcileGraph(graph, draft);
    const elapsed = performance.now() - started;
    expect(descendants(saved, graph.nodes[0].id)).toHaveLength(98);
    expect(saved.nodes.slice(2).every(node => node.stale)).toBe(true);
    expect(saved.nodes.map(node => node.id)).toEqual(graph.nodes.map(node => node.id));
    expect(saved.edges).toEqual(graph.edges);
    expect(graph.nodes.every(node => !node.stale)).toBe(true);
    // A generous regression guard for pure domain work, not a browser p95 claim.
    expect(elapsed).toBeLessThan(1000);
    console.info(`domain-only 100 nodes / 150 edges validation + invalidation: ${elapsed.toFixed(2)} ms`);
  });
  it('unchanged layout cannot fabricate stale clearing or configuration revisions', () => {
    const graph = denseGraph(); graph.nodes[2].stale = true;
    const draft = structuredClone(graph); draft.nodes.forEach(node => { node.position.x += 10; node.configVersion = 9999; node.stale = false; });
    const saved = reconcileGraph(graph, draft);
    expect(saved.nodes.every(node => node.configVersion === 1)).toBe(true);
    expect(saved.nodes[2].stale).toBe(true);
    expect(saved.nodes.filter(node => node.stale)).toHaveLength(1);
  });
});


describe('100 node / 150 edge batch readiness', () => {
  it('resolves all 98 executable nodes together without relaxing graph validation', () => {
    const graph = denseGraph(); graph.nodes.forEach(node => { delete node.selectedAssetVersionId; });
    const project: Project = { id: 'stress', name: 'stress', schemaVersion: 1, graphRevision: 1, layoutRevision: 1, createdAt: '', updatedAt: '', graph };
    const started = performance.now();
    const resolved = resolveGraphInputs(project, [], 'mock');
    const elapsed = performance.now() - started;
    expect(Object.keys(resolved)).toHaveLength(98);
    expect(Object.values(resolved).every(value => value.snapshot?.prompt === 'Stress graph prompt' && !value.error)).toBe(true);
    // Domain-only regression guard; actual browser interaction has separate evidence.
    expect(elapsed).toBeLessThan(250);
    console.info(`domain-only 100 nodes / 150 edges batch readiness: ${elapsed.toFixed(2)} ms`);
  });
});
