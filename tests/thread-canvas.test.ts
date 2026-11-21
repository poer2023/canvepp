import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { mkdtempSync, rmSync } from 'node:fs';
import { randomUUID } from 'node:crypto';
import path from 'node:path';
import os from 'node:os';
import { CanvasStore } from '../src/server/store';
import { openThreadCanvas, threadCanvas, canvasThread } from '../src/server/thread-canvas';
import { registerHostContext, readHostContext, dispatchHostRun } from '../src/server/host-dispatch';

let dir: string, store: CanvasStore;
beforeEach(() => { dir = mkdtempSync(path.join(os.tmpdir(), 'canvepp-thread-')); store = new CanvasStore(dir, { autoRun: false }); });
afterEach(() => { store.close(); rmSync(dir, { recursive: true, force: true }); });

describe('one durable canvas for each Codex chat', () => {
    it('reopens the same canvas through different connections and after a restart', () => {
        const threadId = randomUUID();
        const first = openThreadCanvas(store, threadId);
        const secondStore = new CanvasStore(dir, { autoRun: false });
        expect(openThreadCanvas(secondStore, threadId).project.id).toBe(first.project.id);
        secondStore.close();
        const token = registerHostContext(store, threadId).token;
        expect(readHostContext(store, token)).toMatchObject({ connected: true, projectId: first.project.id });
        store.close(); store = new CanvasStore(dir, { autoRun: false });
        expect(openThreadCanvas(store, threadId)).toEqual(first);
        expect(store.listProjects()).toHaveLength(1);
        expect(store.listRuns(first.project.id)).toEqual([]);
    });
    it('attaches an existing canvas without changing its content, images or revision', async () => {
        const original = await store.createProject('旧画布', true);
        const bytes = store.assetBytes(original.assets[0].id);
        const threadId = randomUUID();
        expect(openThreadCanvas(store, threadId, original.project.id)).toEqual(original);
        expect(store.assetBytes(original.assets[0].id)).toEqual(bytes);
        expect(canvasThread(store, original.project.id)?.threadId).toBe(threadId);
    });
    it('refuses either direction of rebinding and creates no extra project on failure', async () => {
        const a = randomUUID(), b = randomUUID(), first = openThreadCanvas(store, a);
        const legacy = await store.createProject('保留的其他画布');
        expect(() => openThreadCanvas(store, a, legacy.project.id)).toThrow('已有画布');
        expect(() => openThreadCanvas(store, b, first.project.id)).toThrow('另一条');
        expect(store.listProjects()).toHaveLength(2);
        expect(threadCanvas(store, b)).toBeUndefined();
        const second = openThreadCanvas(store, b);
        expect(second.project.id).not.toBe(first.project.id);
        expect(threadCanvas(store, a)?.projectId).toBe(first.project.id);
    });
    it('keeps the durable association when a connection expires', () => {
        const id = randomUUID(), canvas = openThreadCanvas(store, id), old = registerHostContext(store, id);
        store.db.prepare('UPDATE host_contexts SET expires_at=0').run();
        expect(readHostContext(store, old.token)).toEqual({ connected: false });
        const fresh = registerHostContext(store, id);
        expect(readHostContext(store, fresh.token)).toMatchObject({ projectId: canvas.project.id });
    });
    it('blocks a wrong chat before queueing or reserving an image run', async () => {
        const d = await store.createProject('有生成输入的画布', true);
        const a = registerHostContext(store, randomUUID()), b = registerHostContext(store, randomUUID());
        openThreadCanvas(store, a.threadId, d.project.id);
        const node = d.project.graph.nodes.find(n => n.kind === 'generate')!;
        const preview = store.preview(d.project.id, node.id, 0, 'codex');
        const run = store.confirm({ previewId: preview.id, snapshotHash: preview.snapshotHash, confirmationId: randomUUID(), idempotencyKey: randomUUID() });
        let sent = 0;
        await expect(dispatchHostRun(store, run.id, b.token, async () => { sent++; })).rejects.toThrow('另一条');
        expect(sent).toBe(0);
        expect(store.db.prepare('SELECT COUNT(*) AS n FROM host_dispatches').get()).toEqual({ n: 0 });
        await dispatchHostRun(store, run.id, a.token, async (thread) => { expect(thread).toBe(a.threadId); sent++; });
        expect(sent).toBe(1);
    });
});

describe('backup restoration retains the destination and its thread', () => {
    it('adds a remapped copy of cards, originals and run history without replacing existing content', async () => {
        const destination = await store.createProject('当前作品', true), source = await store.createProject('备份作品', true);
        const threadId = randomUUID(); openThreadCanvas(store, threadId, destination.project.id);
        const node = source.project.graph.nodes.find(n => n.kind === 'generate')!;
        const preview = store.preview(source.project.id, node.id, 0, 'mock');
        const run = store.confirm({ previewId: preview.id, snapshotHash: preview.snapshotHash, confirmationId: randomUUID(), idempotencyKey: randomUUID() });
        store.cancel(run.id);
        const sourceBefore = store.detail(source.project.id), zip = await store.exportProject(source.project.id);
        const restored = await store.importProject(zip, { projectId: destination.project.id, expectedRevision: 0 });
        expect(restored.project.id).toBe(destination.project.id);
        expect(restored.project.name).toBe(destination.project.name);
        expect(restored.project.graph.nodes.slice(0, destination.project.graph.nodes.length)).toEqual(destination.project.graph.nodes);
        expect(restored.assets.slice(0, destination.assets.length)).toEqual(destination.assets);
        expect(restored.project.graph.nodes).toHaveLength(destination.project.graph.nodes.length + source.project.graph.nodes.length);
        expect(restored.runs[0].id).not.toBe(run.id);
        expect(restored.runs[0].inputSnapshot.prompt).toBe(run.inputSnapshot.prompt);
        expect(restored.runs[0].status).toBe('cancelled');
        expect(threadCanvas(store, threadId)?.projectId).toBe(destination.project.id);
        expect(store.detail(source.project.id)).toEqual(sourceBefore);
        expect(store.listProjects()).toHaveLength(2);
        for (const asset of restored.assets) expect(store.assetBytes(asset.id).byteLength).toBeGreaterThan(0);
    });
    it('rejects a stale revision or invalid archive without touching saved records', async () => {
        const destination = await store.createProject('保留', true), source = await store.createProject('源', true);
        const zip = await store.exportProject(source.project.id);
        await expect(store.importProject(zip, { projectId: destination.project.id, expectedRevision: 99 })).rejects.toThrow('已更新');
        await expect(store.importProject(Buffer.from('invalid'), { projectId: destination.project.id, expectedRevision: 0 })).rejects.toThrow('无法读取');
        expect(store.detail(destination.project.id)).toEqual(destination);
        expect(store.listProjects()).toHaveLength(2);
    });
});
