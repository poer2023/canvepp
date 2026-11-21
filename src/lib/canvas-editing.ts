import type { CanvasEdge, CanvasNode, Graph } from '../domain/types';
import { DEFAULT_PARAMS } from '../domain/graph';

interface ClipboardInput {
    edge: CanvasEdge;
    /** Only the source's output is reused; its execution history is never copied. */
    source: CanvasNode;
}

export interface CanvasClipboard {
    version: 1;
    projectId: string;
    nodes: CanvasNode[];
    edges: CanvasEdge[];
    incoming: ClipboardInput[];
}

export interface PasteSelectionOptions {
    projectId: string;
    /** One-based ordinal, reset by the caller whenever a new selection is copied. */
    pasteIndex?: number;
    /** An explicit translation, otherwise each paste moves 48px along both axes. */
    offset?: { x: number; y: number };
    idFactory?: () => string;
    /** Supply only IDs returned by a completed import into the target project. */
    assetVersionMap?: Readonly<Record<string, string>>;
}

export interface PasteSelectionResult {
    graph: Graph;
    /** Copies of the selected nodes, excluding any restored input dependencies. */
    nodeIds: string[];
    edgeIds: string[];
    dependencyNodeIds: string[];
}

const clone = <T>(value: T): T => structuredClone(value);
const imageOutput = (node: CanvasNode) => ['image', 'generate', 'edit'].includes(node.kind);
const copyName = (name: string, suffix = ' 副本') => name.slice(0, 200 - suffix.length) + suffix;

/** Capture nodes, internal edges and incoming output values at copy time. */
export function copySelection(graph: Graph, projectId: string, nodeIds: readonly string[]): CanvasClipboard | undefined {
    const selected = new Set(nodeIds);
    const nodes = graph.nodes.filter(node => selected.has(node.id));
    if (!nodes.length) return;
    if (nodes.some(node => node.projectId !== projectId)) throw new Error('不能复制其他项目的节点');
    const included = new Set(nodes.map(node => node.id));
    const sources = new Map(graph.nodes.map(node => [node.id, node]));
    const edges = graph.edges.filter(edge => included.has(edge.sourceNodeId) && included.has(edge.targetNodeId));
    const incoming = graph.edges.filter(edge => !included.has(edge.sourceNodeId) && included.has(edge.targetNodeId)).map(edge => {
        const source = sources.get(edge.sourceNodeId);
        if (!source || source.projectId !== projectId) throw new Error('复制的输入连线缺少同项目来源');
        return { edge, source };
    });
    return clone({ version: 1, projectId, nodes, edges, incoming });
}

function sameOutput(current: CanvasNode, captured: CanvasNode): boolean {
    if (current.kind === 'text' && captured.kind === 'text') return (current.config.text ?? '') === (captured.config.text ?? '');
    if (current.kind === 'params' && captured.kind === 'params') {
        const a = current.config.params, b = captured.config.params;
        return a === undefined || b === undefined ? a === b : a.size === b.size && a.quality === b.quality && a.n === b.n;
    }
    return imageOutput(current) && imageOutput(captured) && current.selectedAssetVersionId === captured.selectedAssetVersionId;
}

/**
 * Paste an immutable snapshot. Same-project incoming links may share an unchanged
 * source. Changed/missing sources are restored as value-only inputs, including an
 * Image node for a captured generated result. No Run, Job or provider call exists
 * in this operation. Foreign image references require an explicit import map.
 */
export function pasteSelection(graph: Graph, clipboard: CanvasClipboard | undefined, options: PasteSelectionOptions): PasteSelectionResult {
    const empty = { graph, nodeIds: [], edgeIds: [], dependencyNodeIds: [] };
    if (!clipboard?.nodes.length) return empty;
    if (!options.projectId || graph.nodes.some(node => node.projectId !== options.projectId)) throw new Error('粘贴目标项目不一致');
    if (clipboard.version !== 1 || clipboard.nodes.some(node => node.projectId !== clipboard.projectId)) throw new Error('复制快照的项目不一致');
    const pasteIndex = options.pasteIndex ?? 1;
    if (!Number.isInteger(pasteIndex) || pasteIndex < 1) throw new Error('粘贴次数必须为正整数');
    const offset = options.offset ?? { x: 48 * pasteIndex, y: 48 * pasteIndex };
    if (!Number.isFinite(offset.x) || !Number.isFinite(offset.y)) throw new Error('粘贴位置无效');
    const crossProject = clipboard.projectId !== options.projectId;
    const capturedIds = new Set(clipboard.nodes.map(node => node.id));
    if (capturedIds.size !== clipboard.nodes.length
        || clipboard.edges.some(edge => !capturedIds.has(edge.sourceNodeId) || !capturedIds.has(edge.targetNodeId))
        || clipboard.incoming.some(({ edge, source }) => !capturedIds.has(edge.targetNodeId) || capturedIds.has(edge.sourceNodeId)
            || edge.sourceNodeId !== source.id || source.projectId !== clipboard.projectId)) throw new Error('复制快照的连线无效');

    // Check every required mapping before allocating or returning any new content.
    const mappedAsset = (assetId: string | undefined): string | undefined => {
        if (!assetId || !crossProject) return assetId;
        const mapped = Object.hasOwn(options.assetVersionMap ?? {}, assetId) ? options.assetVersionMap?.[assetId] : undefined;
        if (!mapped || mapped === assetId) throw new Error('跨项目粘贴包含图片，请先导入图片；原有图片内容不会被清空');
        return mapped;
    };
    for (const node of [...clipboard.nodes, ...clipboard.incoming.map(input => input.source)]) mappedAsset(node.selectedAssetVersionId);

    const usedIds = new Set([
        ...graph.nodes.map(node => node.id), ...graph.edges.map(edge => edge.id),
        ...clipboard.nodes.map(node => node.id), ...clipboard.edges.map(edge => edge.id),
        ...clipboard.incoming.flatMap(input => [input.edge.id, input.source.id]),
    ]);
    const makeId = options.idFactory ?? (() => crypto.randomUUID());
    const freshId = () => {
        for (let attempt = 0; attempt < 32; attempt++) {
            const value = makeId();
            if (typeof value === 'string' && value && value.length <= 200 && !usedIds.has(value)) { usedIds.add(value); return value; }
        }
        throw new Error('无法分配新的节点或连线标识，粘贴未执行');
    };
    const nodeMap = new Map(clipboard.nodes.map(node => [node.id, freshId()]));
    const translated = (node: CanvasNode) => ({ x: node.position.x + offset.x, y: node.position.y + offset.y });
    const copies = clipboard.nodes.map(node => ({ ...clone(node), id: nodeMap.get(node.id)!, projectId: options.projectId,
        name: copyName(node.name), position: translated(node), configVersion: 1, selectedAssetVersionId: mappedAsset(node.selectedAssetVersionId) }));
    const edges = clipboard.edges.map(edge => ({ ...clone(edge), id: freshId(), sourceNodeId: nodeMap.get(edge.sourceNodeId)!, targetNodeId: nodeMap.get(edge.targetNodeId)! }));
    const currentSources = new Map(graph.nodes.map(node => [node.id, node]));
    const sourceMap = new Map<string, string>();
    const dependencies: CanvasNode[] = [];
    for (const input of clipboard.incoming) {
        const key = `${input.source.id}:${input.edge.sourcePort}`;
        let sourceId = sourceMap.get(key);
        if (!sourceId) {
            const current = currentSources.get(input.source.id);
            if (!crossProject && current && sameOutput(current, input.source)) sourceId = current.id;
            else {
                const source = input.source;
                if (!['text', 'params'].includes(source.kind) && !imageOutput(source)) throw new Error('复制快照的输入类型无效');
                const dependency: CanvasNode = imageOutput(source)
                    ? { id: freshId(), projectId: options.projectId, kind: 'image', name: copyName(source.name, ' 输入副本'),
                        position: translated(source), configVersion: 1, config: { text: '', params: { ...DEFAULT_PARAMS }, ...(source.config.note !== undefined ? { note: source.config.note } : {}) },
                        selectedAssetVersionId: mappedAsset(source.selectedAssetVersionId), stale: source.stale }
                    : { ...clone(source), id: freshId(), projectId: options.projectId, name: copyName(source.name, ' 输入副本'),
                        position: translated(source), configVersion: 1, selectedAssetVersionId: mappedAsset(source.selectedAssetVersionId) };
                dependencies.push(dependency); sourceId = dependency.id;
            }
            sourceMap.set(key, sourceId);
        }
        edges.push({ ...clone(input.edge), id: freshId(), sourceNodeId: sourceId, targetNodeId: nodeMap.get(input.edge.targetNodeId)! });
    }
    // Keep paste within the same local-project limits enforced by Store.patchGraph,
    // including automatically restored boundary inputs. Never return a partial paste.
    if (graph.nodes.length + dependencies.length + copies.length > 500 || graph.edges.length + edges.length > 1000) {
        throw new Error('粘贴后将超过项目上限（500 个节点、1000 条连线），粘贴未执行；原画布与复制内容保持不变');
    }
    return {
        graph: { ...graph, nodes: [...graph.nodes, ...dependencies, ...copies], edges: [...graph.edges, ...edges] },
        nodeIds: copies.map(node => node.id), edgeIds: edges.map(edge => edge.id), dependencyNodeIds: dependencies.map(node => node.id),
    };
}

/** Remove selected nodes and their incident edges, plus any explicitly selected edges. */
export function deleteSelection(graph: Graph, nodeIds: readonly string[], edgeIds: readonly string[] = []) {
    const selectedNodes = new Set(nodeIds), selectedEdges = new Set(edgeIds);
    const nodes = graph.nodes.filter(node => !selectedNodes.has(node.id));
    const edges = graph.edges.filter(edge => !selectedEdges.has(edge.id) && !selectedNodes.has(edge.sourceNodeId) && !selectedNodes.has(edge.targetNodeId));
    return {
        graph: nodes.length === graph.nodes.length && edges.length === graph.edges.length ? graph : { ...graph,
            nodes: nodes.length === graph.nodes.length ? graph.nodes : nodes, edges: edges.length === graph.edges.length ? graph.edges : edges },
        removedNodeIds: graph.nodes.filter(node => selectedNodes.has(node.id)).map(node => node.id),
        removedEdgeIds: graph.edges.filter(edge => selectedEdges.has(edge.id) || selectedNodes.has(edge.sourceNodeId) || selectedNodes.has(edge.targetNodeId)).map(edge => edge.id),
    };
}

/** History restores authored content without moving the current tab's viewport. */
export function restoreGraphSnapshot(current: Graph, snapshot: Graph, projectId = current.nodes[0]?.projectId): Graph {
    if (projectId && snapshot.nodes.some(node => node.projectId !== projectId)) throw new Error('不能恢复其他项目的编辑历史');
    return { nodes: clone(snapshot.nodes), edges: clone(snapshot.edges), ...(current.viewport ? { viewport: clone(current.viewport) } : {}) };
}
