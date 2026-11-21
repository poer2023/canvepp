import type { AssetVersion, CanvasEdge, CanvasNode, ImageParams, InputSnapshot, Project, Run, RunMode } from './types';

export interface GenerationGroupPreview {
    id: string;
    projectId: string;
    graphRevision: number;
    mode: RunMode;
    execution?: 'canvas';
    sourceNodeIds: string[];
    prompt: string;
    params: ImageParams;
    references: AssetVersion[];
    nodes: CanvasNode[];
    edges: CanvasEdge[];
    snapshots: InputSnapshot[];
    snapshotHash: string;
    expiresAt: string;
}
export interface GenerationGroupResult { id: string; project: Project; runs: Run[]; }
export interface GenerationGroupInfo { id: string; index: number; total: number; sourceNodeIds: string[]; referenceCount: number; resultCard: true; }
export function generationGroup(run: Run): GenerationGroupInfo | undefined {
    const value = run.usage?.generationGroup as GenerationGroupInfo | undefined;
    return value?.resultCard === true && typeof value.id === 'string' && Number.isInteger(value.index) && Number.isInteger(value.total) && Array.isArray(value.sourceNodeIds) && value.sourceNodeIds.every(id=>typeof id==='string') ? value : undefined;
}
export const snapshotSources = (snapshot: InputSnapshot) => snapshot.sourceAssetVersionIds ?? (snapshot.sourceAssetVersionId ? [snapshot.sourceAssetVersionId] : []);
