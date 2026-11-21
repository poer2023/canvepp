import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { mkdtempSync, rmSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import sharp from 'sharp';
import { CanvasStore } from '../src/server/store';
import { searchLibrary } from '../src/server/image-library';
import { filterLibrary, localLibrary } from '../src/domain/image-library';

let store: CanvasStore, dir: string;
beforeEach(() => { dir = mkdtempSync(path.join(os.tmpdir(), 'canvepp-gallery-')); store = new CanvasStore(dir, { autoRun: false, mockDelayMs: 0 }); });
afterEach(() => { store.close(); rmSync(dir, { recursive: true, force: true }); });
const search = (params: Record<string, string> = {}) => searchLibrary(store, new URLSearchParams(params));
async function fixture() {
    const first = await store.createProject('海边练习', false, 'image');
    const node = first.project.graph.nodes[0];
    node.config.text = '落日灯塔 unique sunset';
    const saved = store.patchGraph(first.project.id, 0, first.project.graph);
    const preview = store.preview(saved.id, node.id, saved.graphRevision, 'mock');
    const run = store.confirm({ previewId: preview.id, snapshotHash: preview.snapshotHash, confirmationId: 'test-confirm', idempotencyKey: 'test-key' });
    const done = await store.processRun(run.id);
    const asset = store.getAsset(done.assetVersionIds[0]);
    return { project: store.getProject(saved.id), asset, node };
}
describe('retained image library', () => {
    it('finds immutable generation prompts after card edits and removal without adopting results', async () => {
        const { project, asset, node } = await fixture();
        expect(search({ q: '落日灯塔' }).items[0]).toMatchObject({ asset: { id: asset.id }, used: false, nodeId: node.id });
        node.config.text = 'new prompt';
        store.patchGraph(project.id, project.graphRevision, { ...project.graph, nodes: [] });
        const before = store.detail(project.id);
        expect(search({ q: 'UNIQUE SUNSET' }).items[0]).toMatchObject({ asset: { id: asset.id }, used: false });
        expect(search().items[0].nodeId).toBeUndefined();
        expect(store.detail(project.id)).toEqual(before);
        expect(store.assetBytes(asset.id).length).toBeGreaterThan(0);
    });
    it('searches all projects with literal wildcard characters and stable pagination', async () => {
        const { project } = await fixture();
        const other = await store.createProject('另一张画布', false, 'image');
        const bytes = await sharp({ create: { width: 64, height: 64, channels: 3, background: '#ccc' } }).png().toBuffer();
        const asset = await store.importAsset(bytes, other.project.id, '100%_done.png', 'strip', other.project.graph.nodes[0].id);
        expect(search().total).toBe(2);
        expect(search({ q: '%_' }).items.map(item => item.asset.id)).toEqual([asset.id]);
        expect(search({ q: '另一张画布' }).items[0].asset.projectId).toBe(other.project.id);
        const all = search().items.map(item => item.asset.id);
        expect([...search({ limit: '1' }).items, ...search({ limit: '1', offset: '1' }).items].map(item => item.asset.id)).toEqual(all);
        expect(search({ since: '2099-01-01' }).total).toBe(0);
        expect(store.getProject(project.id).graph.nodes).toHaveLength(1);
    });
    it('keeps local filters consistent with the saved library and locates a copied surviving card', async () => {
        const { project, asset } = await fixture();
        const original = project.graph.nodes[0];
        const graph = { ...project.graph, nodes: [{ ...original, id: 'surviving-copy', selectedAssetVersionId: asset.id }] };
        const updated = store.patchGraph(project.id, project.graphRevision, graph);
        const detail = store.detail(project.id);
        const local = localLibrary(detail.assets, detail.runs, updated.graph, updated.name);
        expect(filterLibrary(local, '落日灯塔')).toEqual(search({ q: '落日灯塔' }).items);
        expect(local[0]).toMatchObject({ nodeId: 'surviving-copy', used: true });
    });
    it('rejects malformed queries instead of silently dropping parts of the library', () => {
        const invalid: Record<string, string>[] = [{ offset: '-1' }, { limit: '101' }, { limit: 'NaN' }, { since: 'not-a-date' }, { q: 'a'.repeat(301) }];
        for (const params of invalid) expect(() => search(params)).toThrow();
    });
});
