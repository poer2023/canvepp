import type { CanvasNode, Graph, ImageParams, NodeKind } from '@/domain/types';
import { DEFAULT_PARAMS } from '@/domain/graph';

export const isImageCreator = (kind: NodeKind) => kind === 'image' || kind === 'generate' || kind === 'edit';

export const PLACEMENT_CARD_SIZE = { width: 246, height: 420 };
const PLACEMENT_GAP = 28;
type Point = { x: number; y: number };
type Measurements = Record<string, { width: number; height: number }>;

/** Reserve an entire result row so obstacles cannot scatter a batch's cards. */
export function findOpenRow(nodes: readonly CanvasNode[], preferred: Point, count: number): Point[] {
    const step = 290, width = (count - 1) * step + PLACEMENT_CARD_SIZE.width, height = PLACEMENT_CARD_SIZE.height;
    const free = (point: Point) => nodes.every(node => point.x + width + PLACEMENT_GAP <= node.position.x || point.x >= node.position.x + PLACEMENT_CARD_SIZE.width + PLACEMENT_GAP || point.y + height + PLACEMENT_GAP <= node.position.y || point.y >= node.position.y + height + PLACEMENT_GAP);
    let anchor: Point | undefined;
    for (let ring = 0; ring <= 64 && !anchor; ring++) {
        const candidates = [{x:preferred.x+ring*step,y:preferred.y}, {x:preferred.x,y:preferred.y+ring*(height+PLACEMENT_GAP)}, {x:preferred.x,y:preferred.y-ring*(height+PLACEMENT_GAP)}];
        anchor = candidates.find(free);
    }
    anchor ??= { x: Math.max(preferred.x,...nodes.map(node=>node.position.x+PLACEMENT_CARD_SIZE.width+PLACEMENT_GAP)), y: preferred.y };
    return Array.from({length:count},(_,index)=>({x:anchor!.x+index*step,y:anchor!.y}));
}

/** Find a nearby free slot without changing any existing node's position. */
export function findOpenPosition(nodes: readonly CanvasNode[], preferred: Point, measurements: Measurements = {}): Point {
    const { width, height } = PLACEMENT_CARD_SIZE;
    const occupied = nodes.map(node => ({
        ...node.position,
        width: Math.max(width, measurements[node.id]?.width || 0),
        height: Math.max(height, measurements[node.id]?.height || 0),
    }));
    const free = (point: Point) => occupied.every(rect =>
        point.x + width + PLACEMENT_GAP <= rect.x || point.x >= rect.x + rect.width + PLACEMENT_GAP ||
        point.y + height + PLACEMENT_GAP <= rect.y || point.y >= rect.y + rect.height + PLACEMENT_GAP);
    if (free(preferred)) return { ...preferred };
    const stepX = width + PLACEMENT_GAP, stepY = height + PLACEMENT_GAP;
    for (let ring = 1; ring <= 64; ring++) {
        const candidates: Point[] = [];
        for (let dx = -ring; dx <= ring; dx++) for (let dy = -ring; dy <= ring; dy++) {
            if (Math.max(Math.abs(dx), Math.abs(dy)) !== ring) continue;
            candidates.push({ x: preferred.x + dx * stepX, y: preferred.y + dy * stepY });
        }
        candidates.sort((a, b) =>
            (a.x - preferred.x) ** 2 + (a.y - preferred.y) ** 2 - (b.x - preferred.x) ** 2 - (b.y - preferred.y) ** 2 || b.x - a.x);
        const slot = candidates.find(free);
        if (slot) return slot;
    }
    return { x: Math.max(...occupied.map(rect => rect.x + rect.width)) + PLACEMENT_GAP, y: preferred.y };
}

/** Connected values keep precedence until this one input is explicitly detached. */
export function getEditorInputs(graph: Graph, node: CanvasNode) {
    const promptEdge = graph.edges.find(edge => edge.targetNodeId === node.id && edge.targetPort === 'prompt');
    const paramsEdge = graph.edges.find(edge => edge.targetNodeId === node.id && edge.targetPort === 'params');
    const promptSource = graph.nodes.find(item => item.id === promptEdge?.sourceNodeId);
    const paramsSource = graph.nodes.find(item => item.id === paramsEdge?.sourceNodeId);
    return {
        promptEdge, paramsEdge, promptSource, paramsSource,
        prompt: promptEdge ? promptSource?.config.text || '' : node.config.text || '',
        params: paramsSource?.config.params ?? node.config.params ?? DEFAULT_PARAMS,
    };
}

/** Copy the effective input without changing a shared upstream node. */
export function makeInputIndependent(graph: Graph, nodeId: string, port: 'prompt' | 'params'): Graph {
    const node = graph.nodes.find(item => item.id === nodeId);
    if (!node) return graph;
    const inputs = getEditorInputs(graph, node);
    const edge = port === 'prompt' ? inputs.promptEdge : inputs.paramsEdge;
    if (!edge) return graph;
    const config = port === 'prompt'
        ? { ...node.config, text: inputs.prompt }
        : { ...node.config, params: { ...inputs.params } as ImageParams };
    return {
        ...graph,
        nodes: graph.nodes.map(item => item.id === nodeId ? { ...item, config } : item),
        edges: graph.edges.filter(item => item.id !== edge.id),
    };
}
