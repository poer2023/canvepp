import { describe, expect, it } from 'vitest';
import { createNode, getNodeOutputAsset, resolveInput, validateGraph } from '../src/domain/graph';
import type { AssetVersion, CanvasEdge, Graph, Project } from '../src/domain/types';
import { copySelection, deleteSelection, pasteSelection, restoreGraphSnapshot } from '../src/lib/canvas-editing';
import { getEditorInputs } from '../src/lib/image-editor';

const projectId = 'editing-project';
const project = (graph: Graph, id = projectId): Project => ({ id, name: 'editing', graph, schemaVersion: 1, graphRevision: 1, layoutRevision: 1, createdAt: '', updatedAt: '' });
const edge = (id: string, sourceNodeId: string, targetNodeId: string, targetPort: string): CanvasEdge => ({ id, sourceNodeId, sourcePort: 'out', targetNodeId, targetPort });
const ids = (prefix = 'copy') => { let next = 0; return () => `${prefix}-${++next}`; };
const asset = (id: string, owner = projectId): AssetVersion => ({ id, projectId: owner, assetId: id, sha256: 'a'.repeat(64), mime: 'image/png', width: 1024, height: 1024, byteSize: 100, createdAt: '', name: id, parentVersionIds: [], source: 'import' });

function fixture() {
    const prompt = createNode('text', projectId, { x: 20, y: 30 }); prompt.id = 'prompt'; prompt.config.text = 'Copy-time prompt';
    const params = createNode('params', projectId, { x: 20, y: 300 }); params.id = 'params'; params.config.params = { size: '1920x1080', quality: 'high', n: 2 };
    const source = createNode('generate', projectId, { x: 300, y: 30 }); source.id = 'source'; source.config.text = 'Source prompt'; source.selectedAssetVersionId = 'source-image';
    const edit = createNode('edit', projectId, { x: 600, y: 30 }); edit.id = 'edit'; edit.config.text = 'Inactive local prompt'; edit.selectedAssetVersionId = 'selected-result'; edit.stale = true;
    const output = createNode('output', projectId, { x: 900, y: 30 }); output.id = 'output';
    const outside = createNode('edit', projectId, { x: 900, y: 400 }); outside.id = 'outside';
    const graph: Graph = { nodes: [prompt, params, source, edit, output, outside], edges: [
        edge('prompt-in', prompt.id, edit.id, 'prompt'), edge('params-in', params.id, edit.id, 'params'),
        edge('image-in', source.id, edit.id, 'image'), edge('internal', edit.id, output.id, 'image'), edge('outgoing', edit.id, outside.id, 'image'),
    ], viewport: { x: 10, y: 20, zoom: 1 } };
    return { graph, prompt, params, source, edit, output, outside };
}

describe('copy and paste canvas content', () => {
    it('uses copy-time node, internal-edge and input snapshots after originals are changed or removed', () => {
        const { graph, prompt, params, source, edit, output } = fixture();
        const clipboard = copySelection(graph, projectId, [edit.id, output.id])!;
        expect(clipboard.edges.map(item => item.id)).toEqual(['internal']);
        expect(clipboard.incoming).toHaveLength(3);
        prompt.config.text = 'Later prompt'; params.config.params!.n = 4; source.selectedAssetVersionId = 'later-image';
        edit.config.text = 'Later local prompt'; edit.selectedAssetVersionId = undefined; graph.edges.length = 0;
        const current: Graph = { nodes: [], edges: [], viewport: { x: 50, y: -20, zoom: 2 } };
        const before = structuredClone(current), captured = structuredClone(clipboard);
        const pasted = pasteSelection(current, clipboard, { projectId, idFactory: ids() });
        expect(current).toEqual(before); expect(clipboard).toEqual(captured);
        expect(pasted.nodeIds).toHaveLength(2); expect(pasted.dependencyNodeIds).toHaveLength(3); expect(pasted.edgeIds).toHaveLength(4);
        const copiedEdit = pasted.graph.nodes.find(node => node.id === pasted.nodeIds[0])!;
        expect(copiedEdit.kind).toBe('edit'); expect(copiedEdit.selectedAssetVersionId).toBe('selected-result');
        expect(copiedEdit.config.text).toBe('Inactive local prompt'); expect(copiedEdit.stale).toBe(true);
        const input = resolveInput(project(pasted.graph), copiedEdit.id, [asset('source-image'), asset('selected-result')], 'mock');
        expect(input.prompt).toBe('Copy-time prompt'); expect(input.params).toEqual({ size: '1920x1080', quality: 'high', n: 2 });
        expect(input.sourceAssetVersionId).toBe('source-image');
        expect(getNodeOutputAsset(pasted.graph, pasted.nodeIds[1])).toBe('selected-result');
        expect(validateGraph(pasted.graph, [asset('source-image'), asset('selected-result')]).valid).toBe(true);
        expect(pasted.graph.edges.every(item => pasted.graph.nodes.some(node => node.id === item.sourceNodeId) && pasted.graph.nodes.some(node => node.id === item.targetNodeId))).toBe(true);
        expect(pasted.graph.nodes.some(node => node.id === 'outside')).toBe(false);
        expect(pasted.graph).not.toHaveProperty('runs'); expect(copiedEdit).not.toHaveProperty('jobId');
    });

    it('reuses unchanged same-project input sources and never copies outgoing links', () => {
        const { graph, edit, output } = fixture();
        const before = structuredClone(graph), clipboard = copySelection(graph, projectId, [output.id, edit.id])!;
        const pasted = pasteSelection(graph, clipboard, { projectId, idFactory: ids() });
        expect(graph).toEqual(before); expect(pasted.dependencyNodeIds).toEqual([]);
        const incoming = pasted.graph.edges.filter(item => item.targetNodeId === pasted.nodeIds[0]);
        expect(incoming.map(item => item.sourceNodeId)).toEqual(['prompt', 'params', 'source']);
        expect(pasted.graph.edges.filter(item => item.sourceNodeId === pasted.nodeIds[0]).map(item => item.targetNodeId)).toEqual([pasted.nodeIds[1]]);
        expect(pasted.graph.nodes.slice(0, graph.nodes.length)).toEqual(graph.nodes);
        expect(new Set(pasted.graph.nodes.map(node => node.id)).size).toBe(pasted.graph.nodes.length);
        expect(new Set(pasted.graph.edges.map(item => item.id)).size).toBe(pasted.graph.edges.length);
    });

    it('restores changed shared input values once without changing either live source or selected node kind', () => {
        const prompt = createNode('text', projectId); prompt.config.text = 'Shared copy-time prompt';
        const first = createNode('generate', projectId), second = createNode('generate', projectId);
        const graph: Graph = { nodes: [prompt, first, second], edges: [edge('one', prompt.id, first.id, 'prompt'), edge('two', prompt.id, second.id, 'prompt')] };
        const clipboard = copySelection(graph, projectId, [first.id, second.id])!;
        prompt.config.text = 'Changed after copying';
        const pasted = pasteSelection(graph, clipboard, { projectId, idFactory: ids() });
        expect(pasted.dependencyNodeIds).toHaveLength(1);
        expect(prompt.config.text).toBe('Changed after copying');
        const restored = pasted.graph.nodes.find(node => node.id === pasted.dependencyNodeIds[0])!;
        expect(restored.kind).toBe('text'); expect(restored.config.text).toBe('Shared copy-time prompt');
        for (const nodeId of pasted.nodeIds) {
            expect(pasted.graph.nodes.find(node => node.id === nodeId)!.kind).toBe('generate');
            expect(resolveInput(project(pasted.graph), nodeId, [], 'mock').prompt).toBe('Shared copy-time prompt');
        }
    });

    it('keeps empty linked prompts authoritative and missing linked params falling back to the copied local values', () => {
        const prompt = createNode('text', projectId); prompt.config.text = '';
        const params = createNode('params', projectId); delete params.config.params;
        const target = createNode('generate', projectId); target.config.text = 'Must stay inactive'; target.config.params = { size: '1536x1024', quality: 'low', n: 3 };
        const graph: Graph = { nodes: [prompt, params, target], edges: [edge('empty', prompt.id, target.id, 'prompt'), edge('fallback', params.id, target.id, 'params')] };
        const clipboard = copySelection(graph, projectId, [target.id])!;
        const pasted = pasteSelection({ nodes: [], edges: [] }, clipboard, { projectId, idFactory: ids() });
        const copied = pasted.graph.nodes.find(node => node.id === pasted.nodeIds[0])!;
        expect(getEditorInputs(pasted.graph, copied).prompt).toBe('');
        expect(getEditorInputs(pasted.graph, copied).params).toEqual(target.config.params);
        expect(() => resolveInput(project(pasted.graph), copied.id, [], 'mock')).toThrow('非空');
    });

    it('preserves chosen image versions for every image-producing kind and all authored configuration', () => {
        const image = createNode('image', projectId), generate = createNode('generate', projectId), edit = createNode('edit', projectId);
        image.selectedAssetVersionId = 'image-version'; generate.selectedAssetVersionId = 'generate-version'; edit.selectedAssetVersionId = 'edit-version';
        generate.config = { text: 'Precise prompt', params: { size: '1792x768', quality: 'high', n: 1 }, note: 'Do not lose this note' };
        const graph: Graph = { nodes: [image, generate, edit], edges: [edge('image-edit', image.id, edit.id, 'image')] };
        const clipboard = copySelection(graph, projectId, graph.nodes.map(node => node.id))!;
        const pasted = pasteSelection(graph, clipboard, { projectId, idFactory: ids() });
        const copies = pasted.nodeIds.map(nodeId => pasted.graph.nodes.find(node => node.id === nodeId)!);
        expect(copies.map(node => node.selectedAssetVersionId)).toEqual(['image-version', 'generate-version', 'edit-version']);
        expect(copies.map(node => node.kind)).toEqual(['image', 'generate', 'edit']);
        expect(copies[1].config).toEqual(generate.config); expect(copies[1].config).not.toBe(generate.config);
        expect(copies[1].config.params).not.toBe(clipboard.nodes[1].config.params);
    });

    it('refuses all foreign image references before making partial copies, or maps every reference after an explicit import', () => {
        const { graph, edit } = fixture();
        const clipboard = copySelection(graph, projectId, [edit.id])!, captured = structuredClone(clipboard);
        const target: Graph = { nodes: [], edges: [] }, before = structuredClone(target);
        let allocated = 0;
        const idFactory = () => `foreign-${++allocated}`;
        expect(() => pasteSelection(target, clipboard, { projectId: 'other-project', idFactory })).toThrow('跨项目');
        expect(() => pasteSelection(target, clipboard, { projectId: 'other-project', idFactory, assetVersionMap: { 'selected-result': 'imported-result' } })).toThrow('跨项目');
        expect(allocated).toBe(0); expect(target).toEqual(before); expect(clipboard).toEqual(captured);
        const pasted = pasteSelection(target, clipboard, { projectId: 'other-project', idFactory, assetVersionMap: { 'selected-result': 'imported-result', 'source-image': 'imported-source' } });
        expect(pasted.graph.nodes.every(node => node.projectId === 'other-project')).toBe(true);
        expect(pasted.graph.nodes.find(node => node.id === pasted.nodeIds[0])!.selectedAssetVersionId).toBe('imported-result');
        const copiedInput = resolveInput(project(pasted.graph, 'other-project'), pasted.nodeIds[0], [asset('imported-result', 'other-project'), asset('imported-source', 'other-project')], 'mock');
        expect(copiedInput.sourceAssetVersionId).toBe('imported-source'); expect(copiedInput.prompt).toBe('Copy-time prompt');
        expect(pasted.graph.nodes.some(node => node.selectedAssetVersionId === 'selected-result' || node.selectedAssetVersionId === 'source-image')).toBe(false);
    });

    it('allows cross-project text content and restores its boundary input instead of connecting to a colliding foreign ID', () => {
        const prompt = createNode('text', projectId); prompt.id = 'shared-id'; prompt.config.text = 'From original project';
        const target = createNode('generate', projectId);
        const graph: Graph = { nodes: [prompt, target], edges: [edge('linked-prompt', prompt.id, target.id, 'prompt')] };
        const foreign = createNode('text', 'other-project'); foreign.id = prompt.id; foreign.config.text = 'Other project content';
        const pasted = pasteSelection({ nodes: [foreign], edges: [] }, copySelection(graph, projectId, [target.id]), { projectId: 'other-project', idFactory: ids() });
        expect(pasted.dependencyNodeIds).toHaveLength(1); expect(foreign.config.text).toBe('Other project content');
        expect(resolveInput(project(pasted.graph, 'other-project'), pasted.nodeIds[0], [], 'mock').prompt).toBe('From original project');
    });

    it('uses repeatable ordinal offsets without moving old nodes or accumulating offsets in the clipboard', () => {
        const first = createNode('text', projectId, { x: -100, y: 30 }), second = createNode('params', projectId, { x: 300, y: 90 });
        first.name = '很'.repeat(200);
        const graph: Graph = { nodes: [first, second], edges: [] }, before = structuredClone(graph);
        const clipboard = copySelection(graph, projectId, [first.id, second.id])!, captured = structuredClone(clipboard);
        const one = pasteSelection(graph, clipboard, { projectId, idFactory: ids('one'), pasteIndex: 1 });
        const two = pasteSelection(one.graph, clipboard, { projectId, idFactory: ids('two'), pasteIndex: 2 });
        const positions = (result: typeof one) => result.nodeIds.map(nodeId => result.graph.nodes.find(node => node.id === nodeId)!.position);
        expect(positions(one)).toEqual([{ x: -52, y: 78 }, { x: 348, y: 138 }]);
        expect(positions(two)).toEqual([{ x: -4, y: 126 }, { x: 396, y: 186 }]);
        expect(two.graph.nodes.slice(0, graph.nodes.length)).toEqual(before.nodes); expect(graph).toEqual(before); expect(clipboard).toEqual(captured);
        expect(two.graph.nodes.every(node => node.name.length <= 200)).toBe(true);
        const explicit = pasteSelection(graph, clipboard, { projectId, idFactory: ids('explicit'), offset: { x: 1000, y: -200 } });
        expect(positions(explicit)).toEqual([{ x: 900, y: -170 }, { x: 1300, y: -110 }]);
    });

    it('does not reuse original identities even after the originals are deleted and fails atomically on an unusable ID factory', () => {
        const { graph, edit, output } = fixture();
        const clipboard = copySelection(graph, projectId, [edit.id, output.id])!, current: Graph = { nodes: [], edges: [] };
        const candidates = [edit.id, 'prompt', 'internal', 'new-one', 'new-one', 'new-two']; let index = 0;
        const pasted = pasteSelection(current, clipboard, { projectId, idFactory: () => candidates[index++] ?? `new-${index}` });
        expect(pasted.nodeIds).toEqual(['new-one', 'new-two']);
        expect(pasted.graph.nodes.some(node => node.id === edit.id || node.id === output.id)).toBe(false);
        expect(() => pasteSelection(current, clipboard, { projectId, idFactory: () => edit.id })).toThrow('标识');
        expect(current).toEqual({ nodes: [], edges: [] });
    });

    it('allows exactly 500 nodes and rejects the entire next paste, counting restored input dependencies', () => {
        const nodes = Array.from({ length: 499 }, (_, index) => ({ ...createNode('text', projectId), id: `existing-${index}` }));
        const graph: Graph = { nodes, edges: [] }, before = structuredClone(graph);
        const clipboard = copySelection(graph, projectId, [nodes[0].id])!, captured = structuredClone(clipboard);
        const atLimit = pasteSelection(graph, clipboard, { projectId, idFactory: ids() });
        expect(atLimit.graph.nodes).toHaveLength(500);
        const atLimitBefore = structuredClone(atLimit.graph);
        expect(() => pasteSelection(atLimit.graph, clipboard, { projectId, idFactory: ids('next') })).toThrow('500 个节点');
        expect(atLimit.graph).toEqual(atLimitBefore); expect(graph).toEqual(before); expect(clipboard).toEqual(captured);

        const source = createNode('text', projectId), target = createNode('generate', projectId);
        source.config.text = 'This missing input also consumes a node slot';
        const withBoundary = copySelection({ nodes: [source, target], edges: [edge('boundary', source.id, target.id, 'prompt')] }, projectId, [target.id])!;
        const boundaryBefore = structuredClone(withBoundary);
        expect(() => pasteSelection(graph, withBoundary, { projectId, idFactory: ids('dependency') })).toThrow('500 个节点');
        expect(graph).toEqual(before); expect(withBoundary).toEqual(boundaryBefore);
        const fits = pasteSelection({ ...graph, nodes: graph.nodes.slice(1) }, withBoundary, { projectId, idFactory: ids('fits') });
        expect(fits.graph.nodes).toHaveLength(500); expect(fits.dependencyNodeIds).toHaveLength(1);
    });

    it('allows exactly 1000 edges and rejects the next incoming edge without modifying either snapshot', () => {
        const prompt = createNode('text', projectId), params = createNode('params', projectId), source = createNode('image', projectId);
        const edits = Array.from({ length: 332 }, () => createNode('edit', projectId));
        const output = createNode('output', projectId);
        const graph: Graph = { nodes: [prompt, params, source, ...edits, output], edges: [
            ...edits.flatMap((node, index) => [edge(`prompt-${index}`, prompt.id, node.id, 'prompt'), edge(`params-${index}`, params.id, node.id, 'params'), edge(`image-${index}`, source.id, node.id, 'image')]),
            edge('output-image', source.id, output.id, 'image'),
        ] };
        expect(graph.edges).toHaveLength(997);
        const clipboard = copySelection(graph, projectId, [edits[0].id])!;
        const atLimit = pasteSelection(graph, clipboard, { projectId, idFactory: ids() });
        expect(atLimit.graph.edges).toHaveLength(1000); expect(atLimit.dependencyNodeIds).toHaveLength(0);
        const oneEdgeClipboard = copySelection(atLimit.graph, projectId, [output.id])!;
        const before = structuredClone(atLimit.graph), captured = structuredClone(oneEdgeClipboard);
        expect(() => pasteSelection(atLimit.graph, oneEdgeClipboard, { projectId, idFactory: ids('over-edge-limit') })).toThrow('1000 条连线');
        expect(atLimit.graph).toEqual(before); expect(oneEdgeClipboard).toEqual(captured);
    });

    it('treats an empty copy as a no-op and rejects invalid project or placement data without mutation', () => {
        const graph: Graph = { nodes: [createNode('text', projectId)], edges: [] }, before = structuredClone(graph);
        expect(copySelection(graph, projectId, ['missing'])).toBeUndefined();
        expect(pasteSelection(graph, undefined, { projectId }).graph).toBe(graph);
        const clipboard = copySelection(graph, projectId, [graph.nodes[0].id]);
        expect(() => copySelection(graph, 'wrong-project', [graph.nodes[0].id])).toThrow('其他项目');
        expect(() => pasteSelection(graph, clipboard, { projectId: 'wrong-project' })).toThrow('目标项目');
        expect(() => pasteSelection(graph, clipboard, { projectId, pasteIndex: 0 })).toThrow('正整数');
        expect(() => pasteSelection(graph, clipboard, { projectId, offset: { x: NaN, y: 0 } })).toThrow('位置');
        expect(graph).toEqual(before);
    });
});

describe('delete and restore canvas content', () => {
    it('deletes nodes with all incident edges and explicitly selected edges while preserving unrelated content', () => {
        const { graph, edit, output, source } = fixture();
        const unrelated = createNode('output', projectId); unrelated.id = 'unrelated-output';
        graph.nodes.push(unrelated); graph.edges.push(edge('explicit-edge', source.id, unrelated.id, 'image'));
        const before = structuredClone(graph);
        const deleted = deleteSelection(graph, [edit.id, 'missing'], ['explicit-edge']);
        expect(deleted.removedNodeIds).toEqual([edit.id]);
        expect(deleted.removedEdgeIds).toEqual(['prompt-in', 'params-in', 'image-in', 'internal', 'outgoing', 'explicit-edge']);
        expect(deleted.graph.nodes.find(node => node.id === source.id)!.selectedAssetVersionId).toBe('source-image');
        expect(deleted.graph.nodes.find(node => node.id === output.id)).toEqual(output);
        expect(deleted.graph.viewport).toEqual(graph.viewport); expect(graph).toEqual(before);
        const edgeOnly = deleteSelection(graph, [], ['internal']);
        expect(edgeOnly.graph.nodes).toBe(graph.nodes); expect(edgeOnly.graph.edges.map(item => item.id)).not.toContain('internal');
        expect(deleteSelection(graph, ['missing'], ['missing']).graph).toBe(graph);
    });

    it('restores copied, deleted and redone content without restoring an old viewport or aliasing history snapshots', () => {
        const { graph, edit, output } = fixture();
        const before = structuredClone(graph);
        const pasted = pasteSelection(graph, copySelection(graph, projectId, [edit.id, output.id]), { projectId, idFactory: ids() }).graph;
        const deleted = deleteSelection(pasted, [edit.id, output.id]).graph;
        deleted.viewport = { x: 300, y: -400, zoom: 4 };
        const undoDelete = restoreGraphSnapshot(deleted, pasted, projectId);
        expect(undoDelete.nodes).toEqual(pasted.nodes); expect(undoDelete.edges).toEqual(pasted.edges); expect(undoDelete.viewport).toEqual(deleted.viewport);
        const undoPaste = restoreGraphSnapshot(undoDelete, before, projectId);
        expect(undoPaste.nodes).toEqual(before.nodes); expect(undoPaste.edges).toEqual(before.edges); expect(undoPaste.viewport).toEqual(deleted.viewport);
        const redoPaste = restoreGraphSnapshot(undoPaste, pasted, projectId);
        expect(redoPaste.nodes.find(node => node.id === edit.id)!.selectedAssetVersionId).toBe('selected-result');
        expect(redoPaste.nodes.find(node => node.id.startsWith('copy-') && node.kind === 'edit')!.selectedAssetVersionId).toBe('selected-result');
        redoPaste.nodes[0].config.text = 'Edited after redo';
        expect(pasted.nodes[0].config.text).toBe('Copy-time prompt'); expect(before.nodes[0].config.text).toBe('Copy-time prompt');
        expect(restoreGraphSnapshot({ nodes: [], edges: [] }, before, projectId).viewport).toBeUndefined();
        expect(() => restoreGraphSnapshot({ nodes: [], edges: [] }, before, 'other-project')).toThrow('其他项目');
    });
});
