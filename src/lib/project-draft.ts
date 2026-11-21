import type { Graph, Project } from '../domain/types';
import { validateGraph } from '../domain/graph';

export type Draft = Pick<Project, 'name' | 'graph'>;

function stable(value: unknown): string {
    if (Array.isArray(value)) return `[${value.map(stable).join(',')}]`;
    if (value && typeof value === 'object') return `{${Object.entries(value).filter(([, v]) => v !== undefined).sort(([a], [b]) => a.localeCompare(b)).map(([k, v]) => `${JSON.stringify(k)}:${stable(v)}`).join(',')}}`;
    return JSON.stringify(value) ?? 'null';
}

/** Viewport and derived metadata are not authored project changes. */
export function draftKey(draft: Draft): string {
    return stable({ name: draft.name, graph: {
        nodes: draft.graph.nodes.map(({ configVersion: _version, stale: _stale, ...node }) => node),
        edges: draft.graph.edges,
    } });
}

export function sameDraft(a: Draft, b: Draft): boolean { return draftKey(a) === draftKey(b); }

export function sharedGraph(graph: Graph): Graph { return { nodes: graph.nodes, edges: graph.edges }; }

interface DraftBackup { version: 1; projectId: string; base: Project; local: Draft; }
const key = (id: string) => `canvepp:draft:${id}`;

function validDraft(value: unknown, projectId: string): value is Draft {
    if (!value || typeof value !== 'object') return false;
    const draft = value as Draft;
    return typeof draft.name === 'string' && !!draft.graph && Array.isArray(draft.graph.nodes) && Array.isArray(draft.graph.edges)
        && draft.graph.nodes.every(node => node && typeof node.id === 'string' && node.projectId === projectId
            && ['text', 'image', 'params', 'generate', 'edit', 'output'].includes(node.kind)
            && typeof node.name === 'string' && node.config && typeof node.config === 'object' && !Array.isArray(node.config)
            && node.position && Number.isFinite(node.position.x) && Number.isFinite(node.position.y))
        && draft.graph.edges.every(edge => edge && ['id', 'sourceNodeId', 'sourcePort', 'targetNodeId', 'targetPort'].every(k => typeof edge[k as keyof typeof edge] === 'string'));
}

// Session storage is isolated per tab: one window must never replace another's
// unsaved draft. Asset bytes remain in the local store; only references go here.
export function readDraftBackup(projectId: string): DraftBackup | undefined {
    try {
        const raw = sessionStorage.getItem(key(projectId));
        if (!raw) return;
        const value = JSON.parse(raw) as DraftBackup;
        if (value.version === 1 && value.projectId === projectId && value.base?.id === projectId
            && Number.isInteger(value.base.graphRevision) && value.base.graphRevision >= 0
            && validDraft(value.base, projectId) && validDraft(value.local, projectId)
            && validateGraph(value.base.graph).valid && validateGraph(value.local.graph).valid) return value;
    } catch { /* A malformed backup cannot replace the durable project. */ }
}

export function writeDraftBackup(base: Project, local: Draft): boolean {
    try {
        if (sameDraft(base, local)) sessionStorage.removeItem(key(base.id));
        else sessionStorage.setItem(key(base.id), JSON.stringify({ version: 1, projectId: base.id, base, local } satisfies DraftBackup));
        return true;
    } catch { return false; }
}

export function readViewport(projectId: string): Graph['viewport'] {
    try {
        const value = JSON.parse(sessionStorage.getItem(`canvepp:viewport:${projectId}`) || 'null');
        if (value && Number.isFinite(value.x) && Number.isFinite(value.y) && Number.isFinite(value.zoom)
            && value.zoom >= .15 && value.zoom <= 4) return value;
    } catch { /* Use the legacy viewport or fit view. */ }
}

export function writeViewport(projectId: string, viewport: NonNullable<Graph['viewport']>): void {
    try { sessionStorage.setItem(`canvepp:viewport:${projectId}`, JSON.stringify(viewport)); } catch { /* Viewing still works when storage is full. */ }
}
