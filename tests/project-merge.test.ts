import { describe, expect, it } from 'vitest';
import { createNode, validateGraph } from '../src/domain/graph';
import type { CanvasEdge, CanvasNode, Graph, NodeKind } from '../src/domain/types';
import { mergeProjectDraft, resolveProjectDraftConflicts, type MergeChoice, type ProjectDraft } from '../src/lib/project-merge';

const clone = <T>(value: T): T => structuredClone(value);
function node(id: string, kind: NodeKind = 'generate'): CanvasNode {
  return { ...createNode(kind, 'project'), id, name: `卡片 ${id}` };
}
function edge(id: string, source: string, target: string, port = 'image'): CanvasEdge {
  return { id, sourceNodeId: source, sourcePort: 'out', targetNodeId: target, targetPort: port };
}
function draft(nodes = [node('a')], edges: CanvasEdge[] = []): ProjectDraft {
  return { name: '项目', graph: { nodes, edges } };
}
function find(value: ProjectDraft, id = 'a'): CanvasNode {
  return value.graph.nodes.find(item => item.id === id)!;
}
function valid(graph: Graph) { expect(validateGraph(graph)).toEqual({ valid: true, errors: [] }); }

describe('project draft three-way merge', () => {
  it('merges a new multi-reference family and completed output without false input conflicts', () => {
    const base=draft([node('a','image'),node('b','image')]),local=clone(base),remote=clone(base);
    find(local,'a').config.note='保留正在输入的备注';
    remote.graph.nodes.push({...node('result'),selectedAssetVersionId:'returned-image'});
    remote.graph.edges.push(edge('ref-2','b','result','reference'),edge('ref-1','a','result','reference'));
    const merged=mergeProjectDraft(base,local,remote);
    expect(merged.conflicts).toEqual([]);expect(merged.merged.graph.edges.map(e=>e.id)).toEqual(['ref-2','ref-1']);
    expect(find(merged.merged,'a').config.note).toBe('保留正在输入的备注');expect(find(merged.merged,'result').selectedAssetVersionId).toBe('returned-image');valid(merged.merged.graph);
    expect(mergeProjectDraft(remote,merged.merged,remote).conflicts).toEqual([]);
  });
  it('merges independent additions to a multi-input port, retaining all references', () => {
    const base=draft([node('a','image'),node('b','image'),node('c','image'),node('result')],[edge('one','a','result','reference')]);
    const local=clone(base),remote=clone(base);local.graph.edges.push(edge('two','b','result','reference'));remote.graph.edges.push(edge('three','c','result','reference'));
    const merged=mergeProjectDraft(base,local,remote);expect(merged.conflicts).toEqual([]);expect(merged.merged.graph.edges).toHaveLength(3);valid(merged.merged.graph);
  });
  it('merges independent project, node, prompt and parameter fields without a conflict', () => {
    const base = draft(), local = clone(base), remote = clone(base);
    local.name = '本地项目名';
    find(local).config.text = '本地提示词';
    find(local).config.params!.size = '1200x800';
    find(remote).name = '远端卡片名';
    find(remote).config.params!.quality = 'high';
    find(remote).config.params!.n = 3;
    const result = mergeProjectDraft(base, local, remote);
    expect(result.conflicts).toEqual([]);
    expect(result.merged.name).toBe(local.name);
    expect(find(result.merged)).toMatchObject({ name: '远端卡片名', config: { text: '本地提示词', params: { size: '1200x800', quality: 'high', n: 3 } } });
    valid(result.merged.graph);
  });

  it('recognizes equal concurrent edits and ignores object key order', () => {
    const base = draft(), local = clone(base), remote = clone(base);
    find(local).config = { text: '相同', params: { n: 2, quality: 'low', size: '800x1200' } };
    find(remote).config = { params: { size: '800x1200', quality: 'low', n: 2 }, text: '相同' };
    expect(mergeProjectDraft(base, local, remote).conflicts).toEqual([]);
  });

  it.each(['base', 'local', 'remote'] as MergeChoice[])('reports a prompt conflict and resolves %s without losing unrelated edits', side => {
    const base = draft(), local = clone(base), remote = clone(base);
    find(base).config.text = '原提示词';
    find(local).config.text = '本地提示词';
    find(remote).config.text = '远端提示词';
    find(local).config.params!.size = '1600x900';
    find(remote).name = '远端命名';
    const pending = mergeProjectDraft(base, local, remote);
    expect(pending.conflicts).toHaveLength(1);
    expect(pending.conflicts[0]).toMatchObject({ path: '/graph/nodes/a/config/text', kind: 'value', base: '原提示词', local: '本地提示词', remote: '远端提示词', nodeId: 'a', nodeName: '卡片 a' });
    expect(pending.conflicts[0].label).toContain('提示词');
    expect(find(pending.merged).config.text).toBe('本地提示词');
    const resolved = resolveProjectDraftConflicts(base, local, remote, { [pending.conflicts[0].path]: side });
    expect(resolved.conflicts).toEqual([]);
    expect(find(resolved.merged).config.text).toBe(find({ base, local, remote }[side]).config.text);
    expect(find(resolved.merged).config.params!.size).toBe('1600x900');
    expect(find(resolved.merged).name).toBe('远端命名');
  });

  it('keeps node position atomic instead of inventing a combined x/y position', () => {
    const base = draft(), local = clone(base), remote = clone(base);
    find(local).position.x = 100;
    find(remote).position.y = 200;
    const result = mergeProjectDraft(base, local, remote);
    expect(result.conflicts.map(value => value.path)).toEqual(['/graph/nodes/a/position']);
    expect(find(result.merged).position).toEqual({ x: 100, y: 0 });
    const resolved = resolveProjectDraftConflicts(base, local, remote, { '/graph/nodes/a/position': 'remote' });
    expect(find(resolved.merged).position).toEqual({ x: 0, y: 200 });
  });

  it('takes derived metadata from remote while ignoring viewport in all three drafts', () => {
    const base = draft(), local = clone(base), remote = clone(base);
    find(local).config.text = '本地修改';
    find(local).configVersion = 100;
    find(local).stale = false;
    find(remote).configVersion = 7;
    find(remote).stale = true;
    base.graph.viewport = { x: 0, y: 0, zoom: 1 };
    local.graph.viewport = { x: 100, y: 30, zoom: 2 };
    remote.graph.viewport = { x: 300, y: 50, zoom: 0 }; // Even a legacy invalid viewport is tab-local.
    const result = mergeProjectDraft(base, local, remote);
    expect(result.conflicts).toEqual([]);
    expect(find(result.merged)).toMatchObject({ configVersion: 7, stale: true, config: { text: '本地修改' } });
    expect(result.merged.graph).not.toHaveProperty('viewport');
  });

  it('does not treat a metadata-only remote update as editing a locally deleted node', () => {
    const base = draft(), local = draft([]), remote = clone(base);
    find(remote).configVersion = 8;
    find(remote).stale = true;
    const result = mergeProjectDraft(base, local, remote);
    expect(result.conflicts).toEqual([]);
    expect(result.merged.graph.nodes).toEqual([]);
  });

  it('preserves concurrent node and edge additions in stable remote-then-local order', () => {
    const base = draft([node('shared', 'text')]), local = clone(base), remote = clone(base);
    local.graph.nodes.push(node('local'));
    remote.graph.nodes.push(node('remote'));
    local.graph.edges.push(edge('local-edge', 'shared', 'local', 'prompt'));
    remote.graph.edges.push(edge('remote-edge', 'shared', 'remote', 'prompt'));
    const result = mergeProjectDraft(base, local, remote);
    expect(result.conflicts).toEqual([]);
    expect(result.merged.graph.nodes.map(value => value.id)).toEqual(['shared', 'remote', 'local']);
    expect(result.merged.graph.edges.map(value => value.id)).toEqual(['remote-edge', 'local-edge']);
    expect(find(result.merged, 'local')).toMatchObject({ configVersion: 1, stale: false });
    valid(result.merged.graph);
    expect(mergeProjectDraft(base, local, remote)).toEqual(result);
  });

  it.each(['local', 'remote'] as const)('reports %s node deletion versus a concurrent edit and permits either choice', deletedSide => {
    const base = draft(), local = clone(base), remote = clone(base);
    const deleted = deletedSide === 'local' ? local : remote, edited = deletedSide === 'local' ? remote : local;
    deleted.graph.nodes = [];
    find(edited).config.text = '编辑尚未丢失';
    const result = mergeProjectDraft(base, local, remote);
    expect(result.conflicts).toHaveLength(1);
    expect(result.conflicts[0]).toMatchObject({ path: '/graph/nodes/a', kind: 'delete_edit' });
    for (const side of ['local', 'remote'] as const) {
      const resolved = resolveProjectDraftConflicts(base, local, remote, { '/graph/nodes/a': side });
      expect(resolved.conflicts).toEqual([]);
      expect(resolved.merged.graph.nodes).toHaveLength(side === deletedSide ? 0 : 1);
      if (side !== deletedSide) expect(find(resolved.merged).config.text).toBe('编辑尚未丢失');
      valid(resolved.merged.graph);
    }
  });

  it('removes unchanged nodes and their incident edges when only one writer deletes them', () => {
    const base = draft([node('image', 'image'), node('out', 'output')], [edge('e', 'image', 'out')]);
    const local = clone(base), remote = clone(base);
    local.graph.nodes = local.graph.nodes.filter(value => value.id !== 'image');
    local.graph.edges = [];
    find(remote, 'out').name = '保留的远端修改';
    const result = mergeProjectDraft(base, local, remote);
    expect(result.conflicts).toEqual([]);
    expect(result.merged.graph.nodes.map(value => value.id)).toEqual(['out']);
    expect(find(result.merged, 'out').name).toBe('保留的远端修改');
    expect(result.merged.graph.edges).toEqual([]);
  });

  it.each(['local', 'remote'] as const)('detects %s deletion versus a newly added dependent edge', deletedSide => {
    const base = draft([node('image', 'image'), node('out', 'output')]);
    const local = clone(base), remote = clone(base);
    const deleted = deletedSide === 'local' ? local : remote, connected = deletedSide === 'local' ? remote : local;
    deleted.graph.nodes = deleted.graph.nodes.filter(value => value.id !== 'image');
    connected.graph.edges = [edge('new-edge', 'image', 'out')];
    find(deleted, 'out').config.note = '独立修改';
    const result = mergeProjectDraft(base, local, remote);
    expect(result.conflicts).toHaveLength(1);
    expect(result.conflicts[0]).toMatchObject({ path: '/graph/nodes/image', kind: 'dangling_edge', relatedNodeIds: ['image'], relatedEdgeIds: ['new-edge'] });
    valid(result.merged.graph);
    for (const side of ['local', 'remote'] as const) {
      const resolved = resolveProjectDraftConflicts(base, local, remote, { '/graph/nodes/image': side });
      expect(resolved.conflicts).toEqual([]);
      expect(resolved.merged.graph.nodes.some(value => value.id === 'image')).toBe(side !== deletedSide);
      expect(resolved.merged.graph.edges).toHaveLength(side === deletedSide ? 0 : 1);
      expect(find(resolved.merged, 'out').config.note).toBe('独立修改');
      valid(resolved.merged.graph);
    }
  });

  it('combines node delete/edit and dependent-edge conflicts into the same resolvable path', () => {
    const base = draft([node('image', 'image'), node('out', 'output')]), local = clone(base), remote = clone(base);
    local.graph.nodes = local.graph.nodes.filter(value => value.id !== 'image');
    find(remote, 'image').name = '远端编辑';
    remote.graph.edges = [edge('e', 'image', 'out')];
    const result = mergeProjectDraft(base, local, remote);
    expect(result.conflicts).toHaveLength(1);
    expect(result.conflicts[0]).toMatchObject({ kind: 'delete_edit', relatedEdgeIds: ['e'] });
    const resolved = resolveProjectDraftConflicts(base, local, remote, { '/graph/nodes/image': 'remote' });
    expect(resolved.conflicts).toEqual([]);
    expect(find(resolved.merged, 'image').name).toBe('远端编辑');
    expect(resolved.merged.graph.edges.map(value => value.id)).toEqual(['e']);
  });

  it('keeps competing input connections as explicit alternatives and always returns a valid candidate', () => {
    const base = draft([node('first', 'text'), node('second', 'text'), node('a')]), local = clone(base), remote = clone(base);
    local.graph.edges = [edge('local-edge', 'first', 'a', 'prompt')];
    remote.graph.edges = [edge('remote-edge', 'second', 'a', 'prompt')];
    find(local).config.params!.quality = 'high';
    find(remote).config.note = '远端备注';
    const result = mergeProjectDraft(base, local, remote);
    expect(result.conflicts).toHaveLength(1);
    expect(result.conflicts[0]).toMatchObject({ path: '/graph/inputs/a/prompt', kind: 'input_edges', local: local.graph.edges, remote: remote.graph.edges });
    expect(result.merged.graph.edges.map(value => value.id)).toEqual(['local-edge']);
    valid(result.merged.graph);
    for (const side of ['base', 'local', 'remote'] as const) {
      const resolved = resolveProjectDraftConflicts(base, local, remote, { '/graph/inputs/a/prompt': side });
      expect(resolved.conflicts).toEqual([]);
      expect(resolved.merged.graph.edges).toEqual({ base, local, remote }[side].graph.edges);
      expect(find(resolved.merged).config).toMatchObject({ note: '远端备注', params: { quality: 'high' } });
      valid(resolved.merged.graph);
    }
  });

  it('treats two retargets of one edge as one conflict rather than combining endpoints', () => {
    const base = draft([node('p1', 'text'), node('p2', 'text'), node('g1'), node('g2')], [edge('e', 'p1', 'g1', 'prompt')]);
    const local = clone(base), remote = clone(base);
    local.graph.edges[0].sourceNodeId = 'p2';
    remote.graph.edges[0].targetNodeId = 'g2';
    const result = mergeProjectDraft(base, local, remote);
    expect(result.conflicts.map(value => value.path)).toEqual(['/graph/edges/e']);
    expect(result.merged.graph.edges[0]).toEqual(local.graph.edges[0]);
    const resolved = resolveProjectDraftConflicts(base, local, remote, { '/graph/edges/e': 'remote' });
    expect(resolved.merged.graph.edges[0]).toEqual(remote.graph.edges[0]);
  });

  it('resolves a joint cycle locally without losing independent additions or field edits', () => {
    const base = draft([node('a', 'edit'), node('b', 'edit')]), local = clone(base), remote = clone(base);
    local.graph.edges = [edge('ab', 'a', 'b')];
    remote.graph.edges = [edge('ba', 'b', 'a')];
    local.graph.nodes.push(node('local-added'));
    remote.graph.nodes.push(node('remote-added'));
    find(local).config.text = '本地文字';
    find(remote, 'b').config.note = '远端备注';
    const result = mergeProjectDraft(base, local, remote);
    expect(result.conflicts).toHaveLength(1);
    expect(result.conflicts[0].kind).toBe('graph');
    expect(result.conflicts[0].path).toMatch(/^\/graph\/topology\//);
    expect(result.merged.graph.edges.map(value => value.id)).toEqual(['ab']);
    for (const side of ['base', 'local', 'remote'] as const) {
      const resolved = resolveProjectDraftConflicts(base, local, remote, { [result.conflicts[0].path]: side });
      expect(resolved.conflicts).toEqual([]);
      expect(resolved.merged.graph.edges).toEqual({ base, local, remote }[side].graph.edges);
      expect(resolved.merged.graph.nodes.map(value => value.id)).toEqual(['a', 'b', 'remote-added', 'local-added']);
      expect(find(resolved.merged).config.text).toBe('本地文字');
      expect(find(resolved.merged, 'b').config.note).toBe('远端备注');
      valid(resolved.merged.graph);
    }
  });

  it('escapes special node IDs in stable JSON Pointer paths', () => {
    const base = draft([node('a/b~c')]), local = clone(base), remote = clone(base);
    find(local, 'a/b~c').name = '本地'; find(remote, 'a/b~c').name = '远端';
    const result = mergeProjectDraft(base, local, remote);
    expect(result.conflicts[0].path).toBe('/graph/nodes/a~1b~0c/name');
    expect(find(resolveProjectDraftConflicts(base, local, remote, { [result.conflicts[0].path]: 'remote' }).merged, 'a/b~c').name).toBe('远端');
  });

  it('resolves a node-kind/port disagreement without discarding independent field edits or components', () => {
    const base = draft([node('a', 'image'), node('out', 'output'), node('independent')]);
    const local = clone(base), remote = clone(base);
    find(local).kind = 'text';
    find(local).config.text = '本地文本';
    find(remote, 'out').config.note = '远端备注';
    find(remote, 'independent').config.params!.size = '1200x800';
    remote.graph.nodes.push(node('remote-added'));
    local.graph.nodes.push(node('local-added'));
    remote.graph.edges = [edge('image-output', 'a', 'out')];
    const result = mergeProjectDraft(base, local, remote);
    expect(result.conflicts).toHaveLength(1);
    expect(result.conflicts[0].path).toMatch(/^\/graph\/ports\//);
    for (const side of ['local', 'remote'] as const) {
      const resolved = resolveProjectDraftConflicts(base, local, remote, { [result.conflicts[0].path]: side });
      expect(resolved.conflicts).toEqual([]);
      expect(find(resolved.merged).kind).toBe(side === 'local' ? 'text' : 'image');
      expect(find(resolved.merged).config.text).toBe('本地文本');
      expect(find(resolved.merged, 'out').config.note).toBe('远端备注');
      expect(find(resolved.merged, 'independent').config.params!.size).toBe('1200x800');
      expect(resolved.merged.graph.nodes.map(value => value.id)).toEqual(['a', 'out', 'independent', 'remote-added', 'local-added']);
      expect(resolved.merged.graph.edges).toHaveLength(side === 'remote' ? 1 : 0);
      valid(resolved.merged.graph);
    }
  });

  it('reports optional field deletion versus an edit without confusing empty and absent values', () => {
    const base = draft(), local = clone(base), remote = clone(base);
    find(base).config.note = '原备注';
    find(remote).config.note = '';
    const result = mergeProjectDraft(base, local, remote);
    expect(result.conflicts[0]).toMatchObject({ path: '/graph/nodes/a/config/note', kind: 'delete_edit', local: undefined, remote: '' });
    expect(find(result.merged).config).not.toHaveProperty('note');
    expect(find(resolveProjectDraftConflicts(base, local, remote, { [result.conflicts[0].path]: 'remote' }).merged).config.note).toBe('');
  });

  it('does not mutate source drafts or expose aliased conflict and merged values', () => {
    const base = draft(), local = clone(base), remote = clone(base);
    find(local).position.x = 5; find(remote).position.y = 6;
    const before = clone({ base, local, remote });
    const result = mergeProjectDraft(base, local, remote);
    find(result.merged).config.params!.n = 4;
    (result.conflicts[0].local as { x: number }).x = 999;
    expect({ base, local, remote }).toEqual(before);
    expect(find(result.merged).position.x).toBe(5);
  });

  it('rejects invalid source topology instead of silently repairing a corrupted draft', () => {
    const base = draft(), local = clone(base), remote = clone(base);
    local.graph.edges = [edge('bad', 'missing', 'a')];
    expect(() => mergeProjectDraft(base, local, remote)).toThrow('合并输入画布无效');
  });
});
