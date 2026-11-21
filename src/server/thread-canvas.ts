import { randomUUID } from 'node:crypto';
import type { Project, ProjectDetail } from '../domain/types';
import { createImageFirstGraph } from '../domain/graph';
import type { CanvasStore } from './store';
import { assert } from './errors';

export interface ThreadCanvasBinding { threadId: string; projectId: string; createdAt: string; }
const initialized = new WeakSet<CanvasStore>();
const UUID = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/i;

function schema(store: CanvasStore) {
    if (initialized.has(store)) return;
    store.db.exec(`CREATE TABLE IF NOT EXISTS thread_canvases (
        thread_id TEXT PRIMARY KEY,
        project_id TEXT NOT NULL UNIQUE REFERENCES projects(id) ON DELETE CASCADE,
        created_at TEXT NOT NULL
    )`);
    initialized.add(store);
}

export function threadCanvas(store: CanvasStore, threadId: string): ThreadCanvasBinding | undefined {
    schema(store);
    return store.db.prepare('SELECT thread_id AS threadId, project_id AS projectId, created_at AS createdAt FROM thread_canvases WHERE thread_id=?').get(threadId) as ThreadCanvasBinding | undefined;
}

export function canvasThread(store: CanvasStore, projectId: string): ThreadCanvasBinding | undefined {
    schema(store);
    return store.db.prepare('SELECT thread_id AS threadId, project_id AS projectId, created_at AS createdAt FROM thread_canvases WHERE project_id=?').get(projectId) as ThreadCanvasBinding | undefined;
}

/** Only an explicit choice can attach an existing canvas. Reopening is idempotent. */
export function openThreadCanvas(store: CanvasStore, threadId: string, projectId?: string): ProjectDetail {
    assert(UUID.test(threadId), '无效 Codex 会话标识');
    if (projectId !== undefined) assert(UUID.test(projectId), '无效画布标识');
    schema(store);
    return store.db.transaction(() => {
        const prior = threadCanvas(store, threadId);
        if (prior) {
            assert(!projectId || projectId === prior.projectId, '这条会话已有画布，请在原画布中恢复备份，或从另一条 Codex 会话打开旧画布。', 409, 'canvas_already_bound');
            return store.detail(prior.projectId);
        }
        let project: Project;
        if (projectId) {
            project = store.getProject(projectId);
            assert(!canvasThread(store, projectId), '这张画布已关联另一条 Codex 会话，请从原会话打开。', 409, 'canvas_already_bound');
        } else {
            const id = randomUUID(), now = new Date().toISOString();
            project = { id, name: '新画布', schemaVersion: 1, graphRevision: 0, layoutRevision: 0, createdAt: now, updatedAt: now, graph: createImageFirstGraph(id) };
            store.db.prepare('INSERT INTO projects VALUES (?,?)').run(id, JSON.stringify(project));
        }
        store.db.prepare('INSERT INTO thread_canvases VALUES (?,?,?)').run(threadId, project.id, new Date().toISOString());
        return store.detail(project.id);
    }).immediate();
}

export function assertCanvasThread(store: CanvasStore, projectId: string, threadId?: string) {
    const binding = canvasThread(store, projectId);
    assert(binding && threadId === binding.threadId, '这张画布属于另一条会话或尚未关联，请从所属 Codex 会话打开后再生成。', 403, 'canvas_thread_mismatch');
    return binding;
}
