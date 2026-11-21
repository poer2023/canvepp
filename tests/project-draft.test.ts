import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createNode } from '../src/domain/graph';
import type { Project } from '../src/domain/types';
import { draftKey, readDraftBackup, readViewport, sameDraft, sharedGraph, writeDraftBackup, writeViewport, type Draft } from '../src/lib/project-draft';

class TabStorage implements Storage {
    private values = new Map<string, string>();
    failWrites = false;
    get length() { return this.values.size; }
    clear() { this.values.clear(); }
    getItem(key: string) { return this.values.get(key) ?? null; }
    key(index: number) { return [...this.values.keys()][index] ?? null; }
    removeItem(key: string) { this.values.delete(key); }
    setItem(key: string, value: string) {
        if (this.failWrites) throw new Error('session storage quota exceeded');
        this.values.set(key, String(value));
    }
}
let tab: TabStorage;
beforeEach(() => { tab = new TabStorage(); vi.stubGlobal('sessionStorage', tab); });
afterEach(() => { vi.unstubAllGlobals(); });
function project(projectId = 'project-one'): Project {
    const node = createNode('generate', projectId, { x: 260, y: 160 });
    node.id = `image-${projectId}`;
    node.config.text = '白色桌面上的透明玻璃杯';
    return { id: projectId, name: '我的画布', schemaVersion: 1, graphRevision: 8, layoutRevision: 2, createdAt: '2026-10-30T00:00:00Z', updatedAt: '2026-10-30T01:00:00Z', graph: { nodes: [node], edges: [], viewport: { x: 12, y: 20, zoom: .8 } } };
}
function edit(base: Project, prompt: string): Draft {
    const local = structuredClone({ name: base.name, graph: base.graph });
    local.graph.nodes[0].config.text = prompt;
    return local;
}
const backupKey = (projectId: string) => `canvepp:draft:${projectId}`;

describe('tab-local authored project drafts', () => {
    it('keeps two tabs edits to the same project independent across reload reads', () => {
        const base = project(), otherTab = new TabStorage();
        const first = edit(base, '第一窗口尚未保存的文字');
        const second = edit(base, '第二窗口自己的草稿');
        expect(writeDraftBackup(base, first)).toBe(true);
        vi.stubGlobal('sessionStorage', otherTab);
        expect(readDraftBackup(base.id)).toBeUndefined();
        expect(writeDraftBackup(base, second)).toBe(true);
        expect(readDraftBackup(base.id)?.local).toEqual(second);
        vi.stubGlobal('sessionStorage', tab);
        expect(readDraftBackup(base.id)?.local).toEqual(first);
        expect(readDraftBackup(base.id)?.base).toEqual(base);
        expect(readDraftBackup(base.id)?.version).toBe(1);
    });

    it('clears only the saved current project backup, preserving other projects, tabs and viewport keys', () => {
        const first = project(), second = project('project-two'), otherTab = new TabStorage();
        writeDraftBackup(first, edit(first, '第一项目草稿'));
        writeDraftBackup(second, edit(second, '第二项目草稿'));
        writeViewport(first.id, { x: 150, y: -40, zoom: 2 });
        tab.setItem('unrelated-session-value', 'keep');
        vi.stubGlobal('sessionStorage', otherTab);
        writeDraftBackup(first, edit(first, '其他窗口的第一项目草稿'));
        const otherBackup = otherTab.getItem(backupKey(first.id));
        vi.stubGlobal('sessionStorage', tab);
        const otherProjectBackup = tab.getItem(backupKey(second.id));
        const acknowledged = structuredClone(first);
        acknowledged.graph.nodes[0].configVersion += 1;
        acknowledged.graph.nodes[0].stale = true;
        acknowledged.graph.viewport = { x: -900, y: 0, zoom: 4 };
        expect(writeDraftBackup(first, acknowledged)).toBe(true);
        expect(readDraftBackup(first.id)).toBeUndefined();
        expect(tab.getItem(backupKey(second.id))).toBe(otherProjectBackup);
        expect(otherTab.getItem(backupKey(first.id))).toBe(otherBackup);
        expect(readViewport(first.id)).toEqual({ x: 150, y: -40, zoom: 2 });
        expect(tab.getItem('unrelated-session-value')).toBe('keep');
    });

    it('rejects malformed restore data so the durable project remains the fallback, retaining the raw backup', () => {
        const durable = project(), valid = { version: 1, projectId: durable.id, base: durable, local: edit(durable, '草稿文字') };
        const malformed = [
            '{invalid json', 'null',
            JSON.stringify({ ...valid, version: 2 }),
            JSON.stringify({ ...valid, projectId: 'other-project' }),
            JSON.stringify({ ...valid, base: { ...durable, id: 'other-project' } }),
            JSON.stringify({ ...valid, base: { id: durable.id } }),
            JSON.stringify({ ...valid, local: { name: 15, graph: { nodes: [], edges: [] } } }),
            JSON.stringify({ ...valid, local: { name: durable.name, graph: { nodes: null, edges: [] } } }),
            JSON.stringify({ ...valid, local: { name: durable.name, graph: { nodes: [null], edges: [] } } }),
        ];
        for (const raw of malformed) {
            tab.setItem(backupKey(durable.id), raw);
            const backup = readDraftBackup(durable.id);
            expect(backup, raw).toBeUndefined();
            expect(backup?.local ?? durable).toEqual(durable);
            expect(tab.getItem(backupKey(durable.id))).toBe(raw);
        }
    });

    it('keeps viewport changes isolated by tab and project without changing authored drafts', () => {
        const first = project(), second = project('project-two'), otherTab = new TabStorage();
        writeDraftBackup(first, edit(first, '保留这段未保存的文字'));
        const backup = tab.getItem(backupKey(first.id));
        writeViewport(first.id, { x: 120, y: -150, zoom: 4 });
        writeViewport(second.id, { x: -33, y: 44, zoom: .15 });
        expect(readViewport(first.id)).toEqual({ x: 120, y: -150, zoom: 4 });
        expect(readViewport(second.id)).toEqual({ x: -33, y: 44, zoom: .15 });
        expect(tab.getItem(backupKey(first.id))).toBe(backup);
        vi.stubGlobal('sessionStorage', otherTab);
        expect(readViewport(first.id)).toBeUndefined();
        writeViewport(first.id, { x: -1, y: -2, zoom: 1 });
        vi.stubGlobal('sessionStorage', tab);
        expect(readViewport(first.id)).toEqual({ x: 120, y: -150, zoom: 4 });
        expect(sharedGraph(first.graph)).toEqual({ nodes: first.graph.nodes, edges: first.graph.edges });
        expect(sharedGraph(first.graph)).not.toHaveProperty('viewport');
    });

    it('ignores malformed or unsupported viewport data without deleting it', () => {
        const viewportKey = 'canvepp:viewport:project-one';
        for (const raw of ['{', 'null', '{}', JSON.stringify({ x: 1, y: 2, zoom: '1' }), JSON.stringify({ x: null, y: 2, zoom: 1 }), JSON.stringify({ x: 1, y: 2, zoom: .14 }), JSON.stringify({ x: 1, y: 2, zoom: 4.01 })]) {
            tab.setItem(viewportKey, raw);
            expect(readViewport('project-one')).toBeUndefined();
            expect(tab.getItem(viewportKey)).toBe(raw);
        }
    });

    it('ignores server metadata and local viewport while preserving authored position and prompt differences', () => {
        const base = project(), metadataOnly = structuredClone(base);
        metadataOnly.graphRevision += 10;
        metadataOnly.layoutRevision += 10;
        metadataOnly.updatedAt = '2026-10-30T02:00:00Z';
        metadataOnly.graph.nodes[0].configVersion += 20;
        metadataOnly.graph.nodes[0].stale = true;
        metadataOnly.graph.viewport = { x: 555, y: -777, zoom: 4 };
        expect(sameDraft(base, metadataOnly)).toBe(true);
        expect(draftKey(base)).toBe(draftKey(metadataOnly));
        const moved = structuredClone(metadataOnly);
        moved.graph.nodes[0].position.x += .25;
        expect(sameDraft(base, moved)).toBe(false);
        const typed = structuredClone(metadataOnly);
        typed.graph.nodes[0].config.text += '，加一束光';
        expect(sameDraft(base, typed)).toBe(false);
        const version = structuredClone(metadataOnly);
        version.graph.nodes[0].selectedAssetVersionId = 'another-immutable-version';
        expect(sameDraft(base, version)).toBe(false);
        expect(sameDraft(base, { ...base, name: '新的画布名称' })).toBe(false);
    });

    it('reports a storage failure without replacing the last recoverable draft', () => {
        const base = project();
        writeDraftBackup(base, edit(base, '先前可恢复的草稿'));
        const before = tab.getItem(backupKey(base.id));
        tab.failWrites = true;
        expect(writeDraftBackup(base, edit(base, '暂时写不下的新文字'))).toBe(false);
        expect(tab.getItem(backupKey(base.id))).toBe(before);
    });
});
