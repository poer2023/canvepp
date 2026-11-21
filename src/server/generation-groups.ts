import { createHash, randomUUID } from 'node:crypto';
import { createNode, resolveInput } from '../domain/graph';
import { generationGroup, type GenerationGroupPreview, type GenerationGroupResult } from '../domain/generation-group';
import type { CanvasEdge, ImageParams, Run, RunMode } from '../domain/types';
import { findOpenRow, getEditorInputs, isImageCreator } from '../lib/image-editor';
import type { CanvasStore } from './store';
import { assert } from './errors';
import { enqueueCanvasExecution } from './canvas-execution';
import { validateProviderInput } from './providers';

const digest = (value: unknown) => createHash('sha256').update(JSON.stringify(value)).digest('hex');
function schema(store: CanvasStore) {
    store.db.exec('CREATE TABLE IF NOT EXISTS generation_previews(id TEXT PRIMARY KEY,body TEXT NOT NULL); CREATE TABLE IF NOT EXISTS generation_groups(id TEXT PRIMARY KEY,preview_id TEXT UNIQUE NOT NULL,confirmation_key TEXT UNIQUE NOT NULL,body TEXT NOT NULL)');
}
export function getGenerationPreview(store: CanvasStore, id: string): GenerationGroupPreview {
    schema(store);
    const row = store.db.prepare('SELECT body FROM generation_previews WHERE id=?').get(id) as { body: string } | undefined;
    assert(row, '找不到生成预览，请重新设置', 404);
    return JSON.parse(row.body);
}
export function previewGenerationGroup(store: CanvasStore, input: { projectId: string; graphRevision: number; mode: RunMode; execution?: 'canvas'; sourceNodeIds: string[]; prompt?: string; params?: ImageParams }): GenerationGroupPreview {
    schema(store);
    const project = store.getProject(input.projectId);
    assert(project.graphRevision === input.graphRevision, '画布已变更，请保存后重新预览', 409, 'conflict');
    assert(['codex','mock','manual'].includes(input.mode), '多张与多图参考请使用 Codex 原生、模拟或手动模式；独立 API 未开放');
    assert(Array.isArray(input.sourceNodeIds) && input.sourceNodeIds.length >= 1 && input.sourceNodeIds.length <= 8 && new Set(input.sourceNodeIds).size === input.sourceNodeIds.length, '请选择 1–8 张不同的图片卡片');
    assert(!input.execution || (input.execution === 'canvas' && input.mode === 'codex'), '独立执行只支持 Codex 原生模式');
    const sources = input.sourceNodeIds.map(id => project.graph.nodes.find(node => node.id === id));
    assert(sources.every(node => node && isImageCreator(node.kind)), '参考卡片不存在或不是图片');
    const sourceNodes = sources as NonNullable<typeof sources[number]>[];
    const multi = sourceNodes.length > 1;
    const controls = getEditorInputs(project.graph, sourceNodes[0]);
    const params = structuredClone(multi ? input.params : controls.params);
    const prompt = multi ? input.prompt : controls.prompt;
    assert(params && Number.isInteger(params.n) && params.n >= 1 && params.n <= 4, '请选择生成 1–4 张');
    assert(typeof prompt === 'string' && prompt.trim().length > 0 && prompt.length <= 32000, '请填写提示词，最多 32000 字');
    assert(project.graph.nodes.length + params.n <= 500, '生成后会超过 500 张卡片的画布上限');
    assert(!store.listRuns(project.id).some(run => !run.historyOnly && (input.sourceNodeIds.includes(run.nodeId) || generationGroup(run)?.sourceNodeIds.some(id => input.sourceNodeIds.includes(id))) && ['queued','submitting','running','awaiting_host','unknown'].includes(run.status)), '所选图片有未完成或待确认的任务，请先处理已有任务');
    let referenceNodes = sourceNodes.filter(node => !!node.selectedAssetVersionId);
    if (multi) assert(referenceNodes.length === sourceNodes.length, '多图参考需要每张卡片都有图片，请移出空白占位图');
    else if (!referenceNodes.length) {
        const node = sourceNodes[0];
        assert(node.kind !== 'image', '请先导入或采用图片');
        referenceNodes = project.graph.edges.filter(edge => edge.targetNodeId === node.id && ['reference','image'].includes(edge.targetPort)).map(edge => project.graph.nodes.find(candidate => candidate.id === edge.sourceNodeId)!);
        if (node.kind === 'edit') assert(referenceNodes.length, '缺少编辑原图');
    }
    const references = referenceNodes.map(node => {
        assert(node?.selectedAssetVersionId, '参考图尚未采用有效图片');
        const asset = store.getAsset(node.selectedAssetVersionId);
        assert(asset.projectId === project.id, '不能引用其他画布的图片');
        store.assetBytes(asset.id);
        return asset;
    });
    const nodes = [], edges: CanvasEdge[] = [], occupied = [...project.graph.nodes];
    const left = Math.min(...sourceNodes.map(node => node.position.x));
    const bottom = Math.max(...sourceNodes.map(node => node.position.y)) + 460;
    const positions = findOpenRow(occupied, {x:left,y:bottom}, params.n);
    for (let i = 0; i < params.n; i++) {
        const node = createNode('generate', project.id, positions[i]);
        node.name = `${multi ? '多图参考' : sourceNodes[0].name} · ${i + 1}/${params.n}`.slice(0, 200);
        node.config = { text: prompt, params: { ...params, n: 1 } };
        nodes.push(node); occupied.push(node);
        for (const reference of referenceNodes) edges.push({ id: randomUUID(), sourceNodeId: reference.id, sourcePort: 'out', targetNodeId: node.id, targetPort: 'reference' });
        if (!multi && !referenceNodes.some(reference => reference.id === sourceNodes[0].id)) edges.push({ id: randomUUID(), sourceNodeId: sourceNodes[0].id, sourcePort: 'out', targetNodeId: node.id, targetPort: 'origin' });
    }
    assert(project.graph.edges.length + edges.length <= 1000, '生成后会超过画布连线上限');
    const planned = { ...project, graph: { ...project.graph, nodes: occupied, edges: [...project.graph.edges, ...edges] } };
    const model = input.mode === 'codex' ? 'codex-native-imagegen' : input.mode === 'mock' ? 'mock-image-v1' : 'manual-external';
    const snapshots = nodes.map(node => { const snapshot = resolveInput(planned, node.id, store.listAssets(project.id), input.mode, model); validateProviderInput(snapshot); return snapshot; });
    const value = { id: randomUUID(), projectId: project.id, graphRevision: project.graphRevision, mode: input.mode, ...(input.execution && { execution: input.execution }), sourceNodeIds: [...input.sourceNodeIds], prompt, params, references, nodes, edges, snapshots, expiresAt: new Date(Date.now() + 300000).toISOString() };
    const preview = { ...value, snapshotHash: digest(value) };
    store.db.prepare('INSERT INTO generation_previews VALUES (?,?)').run(preview.id, JSON.stringify(preview));
    return preview;
}

export function confirmGenerationGroup(store: CanvasStore, input: { previewId: string; snapshotHash: string; confirmationKey: string; acceptStale?: boolean }): GenerationGroupResult {
    schema(store);
    assert(typeof input.confirmationKey === 'string' && input.confirmationKey.length > 0 && input.confirmationKey.length <= 100, '缺少确认标识');
    return store.db.transaction(() => {
        const preview = getGenerationPreview(store, input.previewId);
        assert(input.snapshotHash === preview.snapshotHash, '生成快照校验失败');
        const existing = store.db.prepare('SELECT preview_id,body FROM generation_groups WHERE preview_id=? OR confirmation_key=?').get(preview.id, input.confirmationKey) as { preview_id: string; body: string } | undefined;
        if (existing) {
            assert(existing.preview_id === preview.id, '确认标识已用于另一组生成', 409, 'conflict');
            const saved = JSON.parse(existing.body) as { id: string; runIds: string[] };
            return { id: saved.id, project: store.getProject(preview.projectId), runs: saved.runIds.map(id => store.getRun(id)) };
        }
        assert(Date.parse(preview.expiresAt) > Date.now(), '预览已过期，请重新确认', 409, 'conflict');
        const project = store.getProject(preview.projectId);
        assert(project.graphRevision === preview.graphRevision, '预览后画布已变更，请重新预览', 409, 'conflict');
        const acceptedReferences = !preview.references.length || input.acceptStale;
        assert(acceptedReferences || !preview.snapshots.some(snapshot => snapshot.upstreamStale.length), '请确认继续使用当前参考图版本');
        for (const asset of preview.references) { assert(store.getAsset(asset.id).sha256 === asset.sha256, '参考图校验失败'); store.assetBytes(asset.id); }
        const next = store.patchGraph(project.id, project.graphRevision, { ...project.graph, nodes: [...project.graph.nodes, ...preview.nodes], edges: [...project.graph.edges, ...preview.edges] });
        const groupId = randomUUID();
        const runs = preview.nodes.map((node, index) => {
            const child = store.preview(project.id, node.id, next.graphRevision, preview.mode);
            assert(digest(child.snapshot) === digest(preview.snapshots[index]), '预览后生成输入发生变化', 409, 'conflict');
            const run = store.confirm({ previewId: child.id, snapshotHash: child.snapshotHash, confirmationId: `${input.confirmationKey}:${index}`, idempotencyKey: `${input.confirmationKey}:${index}`, acceptStale: acceptedReferences });
            const updated: Run = { ...run, usage: { ...run.usage, ...(preview.execution && {canvasExecution:{}}), generationGroup: { id: groupId, index, total: preview.params.n, sourceNodeIds: preview.sourceNodeIds, referenceCount: preview.references.length, resultCard: true } } };
            store.db.prepare('UPDATE runs SET body=? WHERE id=?').run(JSON.stringify(updated), run.id);
            if (preview.execution === 'canvas') enqueueCanvasExecution(store, updated);
            return updated;
        });
        store.db.prepare('INSERT INTO generation_groups VALUES (?,?,?,?)').run(groupId, preview.id, input.confirmationKey, JSON.stringify({ id: groupId, runIds: runs.map(run => run.id) }));
        return { id: groupId, project: next, runs };
    }).immediate();
}

/** Only fill a newly authorized, untouched result card. Never alter the source. */
export function publishGenerationResult(store: CanvasStore, run: Run) {
    if (!generationGroup(run) || !run.assetVersionIds.length || run.cancelRequested || run.lateResult || run.draftChanged) return;
    const project = store.getProject(run.projectId), node = project.graph.nodes.find(node => node.id === run.nodeId);
    if (!node || node.selectedAssetVersionId) return;
    let current;
    try { current = resolveInput(project, node.id, store.listAssets(project.id), run.mode, run.inputSnapshot.model); }
    catch { return; } // A removed reference must not prevent retaining a completed image in history.
    if (digest(current) !== digest(run.inputSnapshot)) return;
    store.patchGraph(project.id, project.graphRevision, { ...project.graph, nodes: project.graph.nodes.map(item => item.id === node.id ? { ...item, selectedAssetVersionId: run.assetVersionIds[0] } : item) });
}
