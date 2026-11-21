import { describe, expect, it } from 'vitest';
import { createNode, createStarterGraph, DEFAULT_PARAMS, getNodeOutputAsset, resolveInput, validateGraph } from '../src/domain/graph';
import type { AssetVersion, CanvasEdge, Graph, Project, Run } from '../src/domain/types';
import { convertGraphToImageFirst, ImageFirstConversionError } from '../src/lib/image-first';

const projectId = 'image-first-test';
const project = (graph: Graph): Project => ({ id: projectId, name: 'legacy example', schemaVersion: 1, graphRevision: 4, layoutRevision: 2, createdAt: '', updatedAt: '', graph });
const edge = (sourceNodeId: string, targetNodeId: string, targetPort: string): CanvasEdge => ({ id: `${sourceNodeId}-${targetNodeId}-${targetPort}`, sourceNodeId, sourcePort: 'out', targetNodeId, targetPort });
const asset = (id: string): AssetVersion => ({ id, projectId, assetId: id, sha256: 'a'.repeat(64), mime: 'image/png', width: 1536, height: 1024, byteSize: 200, createdAt: '', name: id, parentVersionIds: [], source: 'import' });

describe('explicit legacy example conversion', () => {
    it('turns a six-node example with results into independent images without losing IDs, layout, notes, effective inputs or visible versions', () => {
        const graph = createStarterGraph(projectId);
        const originalImage = graph.nodes.find(node => node.kind === 'image')!, edit = graph.nodes.find(node => node.kind === 'edit')!, generate = graph.nodes.find(node => node.kind === 'generate')!, output = graph.nodes.find(node => node.kind === 'output')!;
        originalImage.selectedAssetVersionId = 'original'; edit.selectedAssetVersionId = 'edited'; generate.selectedAssetVersionId = 'generated';
        edit.config.note = 'Keep the edit note'; output.config.note = 'Keep only my output note';
        graph.nodes.find(node => node.kind === 'params')!.config.params = { size: '1920x1080', quality: 'high', n: 1 };
        const assets = [asset('original'), { ...asset('edited'), parentVersionIds: ['original'] }, asset('generated')];
        const before = structuredClone(graph), assetsBefore = structuredClone(assets);
        const oldInput = resolveInput(project(graph), edit.id, assets, 'mock');
        const converted = convertGraphToImageFirst(graph, assets);
        expect(converted.graph.nodes).toHaveLength(4); expect(converted.graph.edges).toEqual([]);
        expect(converted.graph.nodes.every(node => node.kind === 'image')).toBe(true);
        for (const node of converted.graph.nodes) {
            const old = before.nodes.find(item => item.id === node.id)!;
            expect(node.position).toEqual(old.position); expect(node.name).toBe(old.name); expect(node.config.note).toBe(old.config.note);
            expect(node.configVersion).toBe(old.configVersion); expect(node.stale).toBe(old.stale);
            expect(node.selectedAssetVersionId).toBe(getNodeOutputAsset(before, old.id));
        }
        const convertedEdit = converted.graph.nodes.find(node => node.id === edit.id)!;
        expect(convertedEdit.config.text).toBe(oldInput.prompt); expect(convertedEdit.config.params).toEqual(oldInput.params);
        const convertedOutput = converted.graph.nodes.find(node => node.id === output.id)!;
        expect(convertedOutput.config.text).toBe(oldInput.prompt); expect(convertedOutput.config.params).toEqual(oldInput.params);
        expect(convertedOutput.selectedAssetVersionId).toBe('edited');
        expect(converted.retainedAssetVersionIds.sort()).toEqual(['edited', 'generated', 'original']);
        expect(converted.provenance.find(item => item.nodeId === edit.id)).toMatchObject({ originalKind: 'edit', kind: 'image', inputSourceNodeId: originalImage.id, originalInputAssetVersionId: 'original', originalVisibleAssetVersionId: 'edited', selectedAssetVersionId: 'edited', usedOriginalInputAsCurrent: false });
        expect(converted.provenance.find(item => item.nodeId === generate.id)!.originalKind).toBe('generate');
        expect(converted.graph.nodes.find(node => node.id === originalImage.id)!.selectedAssetVersionId).toBe('original');
        expect(converted.backup).toEqual(before); expect(graph).toEqual(before); expect(assets).toEqual(assetsBefore);
        expect(validateGraph(converted.graph, assets).valid).toBe(true);
    });

    it('fixes an edit without a result to its exact original input while keeping an originally empty output empty', () => {
        const graph = createStarterGraph(projectId);
        const original = graph.nodes.find(node => node.kind === 'image')!, edit = graph.nodes.find(node => node.kind === 'edit')!, output = graph.nodes.find(node => node.kind === 'output')!;
        original.selectedAssetVersionId = 'original'; edit.config.note = 'Unfinished edit'; output.config.note = 'Empty output';
        const assets = [asset('original')], oldInput = resolveInput(project(graph), edit.id, assets, 'mock');
        expect(getNodeOutputAsset(graph, edit.id)).toBeUndefined(); expect(getNodeOutputAsset(graph, output.id)).toBeUndefined();
        const converted = convertGraphToImageFirst(graph, assets);
        const nextEdit = converted.graph.nodes.find(node => node.id === edit.id)!, nextOutput = converted.graph.nodes.find(node => node.id === output.id)!;
        expect(nextEdit.kind).toBe('image'); expect(nextEdit.selectedAssetVersionId).toBe('original'); expect(nextEdit.config.note).toBe('Unfinished edit');
        const nextInput = resolveInput(project(converted.graph), nextEdit.id, assets, 'mock');
        expect({ prompt: nextInput.prompt, params: nextInput.params, asset: nextInput.sourceAssetVersionId }).toEqual({ prompt: oldInput.prompt, params: oldInput.params, asset: oldInput.sourceAssetVersionId });
        expect(nextOutput.kind).toBe('generate'); expect(nextOutput.selectedAssetVersionId).toBeUndefined();
        expect(nextOutput.config).toEqual({ text: oldInput.prompt, params: oldInput.params, note: 'Empty output' });
        expect(converted.provenance.find(item => item.nodeId === edit.id)).toMatchObject({ originalInputAssetVersionId: 'original', originalVisibleAssetVersionId: undefined, selectedAssetVersionId: 'original', usedOriginalInputAsCurrent: true });
    });

    it('copies an output image and effective controls independently, preserving its own name, position and note', () => {
        const source = createNode('generate', projectId); source.selectedAssetVersionId = 'visible'; source.config = { text: 'Upstream prompt', params: { size: '1792x768', quality: 'low', n: 1 }, note: 'Source-only note' };
        const output = createNode('output', projectId, { x: 123, y: 456 }); output.name = 'Final image'; output.config = { note: 'Output-only note' };
        const graph: Graph = { nodes: [source, output], edges: [edge(source.id, output.id, 'image')] };
        const converted = convertGraphToImageFirst(graph, [asset('visible')]);
        const next = converted.graph.nodes.find(node => node.id === output.id)!;
        expect(next).toMatchObject({ id: output.id, name: output.name, position: output.position, kind: 'image', selectedAssetVersionId: 'visible' });
        expect(next.config).toEqual({ text: 'Upstream prompt', params: source.config.params, note: 'Output-only note' });
        expect(next.config.params).not.toBe(source.config.params);
        next.config.params!.quality = 'high'; next.config.text = 'Changed output only';
        expect(converted.graph.nodes[0].config.text).toBe('Upstream prompt'); expect(converted.graph.nodes[0].config.params!.quality).toBe('low');
        expect(graph.nodes[1].config).toEqual({ note: 'Output-only note' });
    });

    it('keeps empty linked prompts authoritative, preserves params fallback, and archives an inactive local draft', () => {
        const text = createNode('text', projectId); text.config.text = '';
        const params = createNode('params', projectId); delete params.config.params;
        const generate = createNode('generate', projectId); generate.config.text = 'Inactive local draft'; generate.config.params = { size: '1024x1536', quality: 'high', n: 3 };
        const graph: Graph = { nodes: [text, params, generate], edges: [edge(text.id, generate.id, 'prompt'), edge(params.id, generate.id, 'params')] };
        const converted = convertGraphToImageFirst(graph);
        expect(converted.graph.nodes[0].config.text).toBe(''); expect(converted.graph.nodes[0].config.params).toEqual(generate.config.params);
        expect(converted.provenance[0].originalConfig.text).toBe('Inactive local draft');
        expect(converted.backup.nodes.find(node => node.id === generate.id)!.config.text).toBe('Inactive local draft');
        expect(() => resolveInput(project(converted.graph), generate.id, [], 'mock')).toThrow('非空');
    });

    it('refuses orphan input content and unique input notes with actionable node IDs, without changing anything', () => {
        const text = createNode('text', projectId); text.config.text = 'My isolated text';
        const params = createNode('params', projectId);
        const graph: Graph = { nodes: [text, params], edges: [] }, before = structuredClone(graph);
        try { convertGraphToImageFirst(graph); throw new Error('Expected refusal'); }
        catch (error) {
            expect(error).toBeInstanceOf(ImageFirstConversionError);
            expect((error as ImageFirstConversionError).issues.map(issue => issue.nodeId)).toEqual([text.id, params.id]);
        }
        expect(graph).toEqual(before);
        const generate = createNode('generate', projectId); text.config.note = 'Do not discard this note';
        const connected: Graph = { nodes: [text, generate], edges: [edge(text.id, generate.id, 'prompt')] }, connectedBefore = structuredClone(connected);
        expect(() => convertGraphToImageFirst(connected)).toThrow('独有备注'); expect(connected).toEqual(connectedBefore);
    });

    it('refuses unexpected hidden content or image references instead of silently dropping them', () => {
        const text = createNode('text', projectId); text.config.text = 'Connected'; text.config.params = { ...DEFAULT_PARAMS };
        const generate = createNode('generate', projectId);
        const graph: Graph = { nodes: [text, generate], edges: [edge(text.id, generate.id, 'prompt')] };
        expect(() => convertGraphToImageFirst(graph)).toThrow('额外配置');
        delete text.config.params; text.selectedAssetVersionId = 'unexpected-image';
        expect(() => convertGraphToImageFirst(graph, [asset('unexpected-image')])).toThrow('图片版本引用');
        const output = createNode('output', projectId); output.selectedAssetVersionId = 'hidden-output-image';
        expect(() => convertGraphToImageFirst({ nodes: [output], edges: [] }, [asset('hidden-output-image')])).toThrow('独立图片引用');
    });

    it('keeps empty images uploadable, converts source-less edit/output cards to placeholders, and leaves original histories untouched', () => {
        const image = createNode('image', projectId), edit = createNode('edit', projectId), output = createNode('output', projectId);
        edit.config = { text: 'Unfinished task', params: { size: '2048x1024', quality: 'medium', n: 1 }, note: 'Still here' };
        output.config = { text: 'Output draft', params: { size: '1024x1024', quality: 'low', n: 1 }, note: 'Output note' };
        const graph: Graph = { nodes: [image, edit, output], edges: [] };
        const run: Run = { id: 'historic-run', jobId: 'historic-job', projectId, nodeId: edit.id, mode: 'mock', status: 'succeeded', inputSnapshot: { nodeId: edit.id, nodeKind: 'edit', configVersion: 1, prompt: 'Historical prompt', params: { ...DEFAULT_PARAMS }, sourceAssetVersionId: 'historical-original', sourceAssetHash: 'a'.repeat(64), upstreamStale: [], model: 'mock', provider: 'mock' }, fingerprint: 'fingerprint', createdAt: '', updatedAt: '', assetVersionIds: ['historic-result'], events: [] };
        const detail = { project: project(graph), assets: [asset('historical-original'), { ...asset('historic-result'), parentVersionIds: ['historical-original'] }], runs: [run] };
        const before = structuredClone(detail), converted = convertGraphToImageFirst(detail.project.graph, detail.assets);
        expect(converted.graph.nodes.map(node => node.kind)).toEqual(['image', 'generate', 'generate']);
        expect(converted.graph.nodes[1].config).toEqual(edit.config); expect(converted.graph.nodes[2].config).toEqual(output.config);
        expect(converted.warnings).toEqual([{ nodeId: edit.id, message: '未选定结果或可用原图，已保留为待创作图片' }]);
        expect(detail).toEqual(before); expect(detail.runs[0].inputSnapshot.nodeKind).toBe('edit');
    });

    it('produces independent durable backups, is idempotent on image-first graphs, and rejects invalid assets/graphs before conversion', () => {
        const graph = createStarterGraph(projectId), image = graph.nodes.find(node => node.kind === 'image')!;
        image.selectedAssetVersionId = 'original';
        const before = structuredClone(graph), converted = convertGraphToImageFirst(graph, [asset('original')]);
        expect(convertGraphToImageFirst(converted.graph, [asset('original')]).graph).toEqual(converted.graph);
        converted.graph.nodes[0].name = 'Changed after conversion';
        expect(converted.backup).toEqual(before); expect(graph).toEqual(before);
        converted.backup.nodes[0].config.text = 'Changed backup';
        expect(graph).toEqual(before); expect(converted.provenance.some(item => item.prompt === 'Changed backup')).toBe(false);
        expect(() => convertGraphToImageFirst(graph, [])).toThrow('素材版本不存在');
        expect(() => convertGraphToImageFirst({ nodes: graph.nodes, edges: [...graph.edges, edge('missing-source', image.id, 'image')] })).toThrow('节点不存在');
    });
});
