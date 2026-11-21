import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { mkdtempSync, rmSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import type { Project, RunPreview } from '../src/domain/types';
import { CanvasStore } from '../src/server/store';
import { AppError } from '../src/server/errors';

let store: CanvasStore;
let dir: string;
beforeEach(() => {
    dir = mkdtempSync(path.join(os.tmpdir(), 'canvepp-save-regression-'));
    store = new CanvasStore(dir, { autoRun: false });
});
afterEach(() => {
    store.close();
    rmSync(dir, { recursive: true, force: true });
});
async function draft() {
    const { project } = await store.createProject('保存回归', false, 'image');
    const graph = structuredClone(project.graph);
    graph.nodes[0].config.text = '白色桌面上的一只透明玻璃杯';
    return store.patchGraph(project.id, project.graphRevision, graph);
}
function persistedBody(projectId: string) {
    return (store.db.prepare('SELECT body FROM projects WHERE id=?').get(projectId) as { body: string }).body;
}
function writeCount() {
    return (store.db.prepare('SELECT total_changes() AS count').get() as { count: number }).count;
}
function preview(project: Project) {
    return store.preview(project.id, project.graph.nodes[0].id, project.graphRevision, 'manual');
}
function confirm(snapshot: RunPreview) {
    return store.confirm({
        previewId: snapshot.id,
        snapshotHash: snapshot.snapshotHash,
        confirmationId: crypto.randomUUID(),
        idempotencyKey: crypto.randomUUID(),
    });
}
function expectConflict(action: () => unknown) {
    let caught: unknown;
    try { action(); } catch (error) { caught = error; }
    expect(caught).toBeInstanceOf(AppError);
    expect(caught).toMatchObject({ status: 409, errorClass: 'conflict' });
}

describe('project saves distinguish content, layout and local viewport changes', () => {
    it('does not write or advance revisions for repeated identical saves, preserving the confirmation preview', async () => {
        const project = await draft();
        const snapshot = preview(project);
        const before = persistedBody(project.id), writes = writeCount();
        for (let i = 0; i < 10; i++) {
            const saved = store.patchGraph(project.id, project.graphRevision, structuredClone(project.graph), project.name);
            expect(saved).toEqual(project);
        }
        expect(writeCount()).toBe(writes);
        expect(persistedBody(project.id)).toBe(before);
        expect(confirm(snapshot).status).toBe('awaiting_import');
        expect(store.listRuns(project.id)).toHaveLength(1);
    });

    it.each([false, true])('ignores viewport-only updates without writes (legacy viewport: %s)', async legacy => {
        let project = await draft();
        if (legacy) {
            // Simulate an older saved project; new PATCH requests must retain its
            // compatibility view rather than replace it with one tab's viewport.
            project = { ...project, graph: { ...project.graph, viewport: { x: 42, y: -72, zoom: .8 } } };
            store.db.prepare('UPDATE projects SET body=? WHERE id=?').run(JSON.stringify(project), project.id);
        }
        const snapshot = preview(project);
        const before = persistedBody(project.id), writes = writeCount();
        const graph = { ...structuredClone(project.graph), viewport: { x: -380.125, y: 920.75, zoom: 4 } };
        const saved = store.patchGraph(project.id, project.graphRevision, graph);
        expect(saved).toEqual(project);
        expect(saved.graph.viewport).toEqual(project.graph.viewport);
        expect(writeCount()).toBe(writes);
        expect(persistedBody(project.id)).toBe(before);
        expect(confirm(snapshot).status).toBe('awaiting_import');
    });

    it('still rejects a genuinely stale revision even when its graph matches current content', async () => {
        const previous = await draft();
        const graph = structuredClone(previous.graph);
        graph.nodes[0].config.text = '黑色桌面上的白瓷杯';
        const current = store.patchGraph(previous.id, previous.graphRevision, graph);
        expect(current.graphRevision).toBe(previous.graphRevision + 1);
        const before = persistedBody(current.id), writes = writeCount();
        expectConflict(() => store.patchGraph(current.id, previous.graphRevision, structuredClone(current.graph)));
        expectConflict(() => store.patchGraph(current.id, previous.graphRevision, previous.graph));
        expect(persistedBody(current.id)).toBe(before);
        expect(writeCount()).toBe(writes);
    });

    it('invalidates an earlier preview after a real prompt edit and freezes the new prompt on a fresh preview', async () => {
        const previous = await draft();
        const snapshot = preview(previous);
        const graph = structuredClone(previous.graph);
        graph.nodes[0].config.text = '将玻璃杯改成蓝色陶瓷杯';
        const current = store.patchGraph(previous.id, previous.graphRevision, graph);
        expect(current.graphRevision).toBe(previous.graphRevision + 1);
        expect(current.graph.nodes[0].configVersion).toBe(previous.graph.nodes[0].configVersion + 1);
        expect(current.layoutRevision).toBe(previous.layoutRevision);
        expectConflict(() => confirm(snapshot));
        expect(store.listRuns(current.id)).toEqual([]);
        const fresh = preview(current);
        expect(fresh.snapshot.prompt).toBe(graph.nodes[0].config.text);
        expect(fresh.snapshotHash).not.toBe(snapshot.snapshotHash);
        expect(confirm(fresh).inputSnapshot.prompt).toBe(graph.nodes[0].config.text);
    });

    it('saves a real node drag across reopening without changing input versions or adopting a result', async () => {
        const previous = await draft();
        const graph = structuredClone(previous.graph);
        graph.nodes[0].position = { x: graph.nodes[0].position.x + 212.25, y: graph.nodes[0].position.y - 40 };
        graph.viewport = { x: 150, y: 200, zoom: 4 };
        const current = store.patchGraph(previous.id, previous.graphRevision, graph);
        expect(current.graphRevision).toBe(previous.graphRevision + 1);
        expect(current.layoutRevision).toBe(previous.layoutRevision + 1);
        expect(current.graph.nodes[0]).toEqual({ ...previous.graph.nodes[0], position: graph.nodes[0].position });
        expect(current.graph.viewport).toEqual(previous.graph.viewport);
        expect(store.patchGraph(current.id, current.graphRevision, structuredClone(current.graph))).toEqual(current);
        store.close();
        store = new CanvasStore(dir, { autoRun: false });
        expect(store.getProject(current.id)).toEqual(current);
        expect(store.listRuns(current.id)).toEqual([]);
        expect(store.listAssets(current.id)).toEqual([]);
    });
});
