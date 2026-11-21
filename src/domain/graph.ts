import type {
  AssetVersion, CanvasEdge, CanvasNode, Graph, GraphValidation, ImageParams,
  InputSnapshot, NodeKind, PortType, Project, RunMode,
} from './types';
import { validateImageSize } from './image-size';

export const DEFAULT_PARAMS: ImageParams = { size: '1024x1024', quality: 'medium', n: 1 };
export const NODE_LABELS: Record<NodeKind, string> = {
  text: '提示词', image: '参考图片', params: '生成参数', generate: '图像生成', edit: '图像编辑', output: '输出',
};
export interface Port { id: string; type: PortType; required?: boolean; multiple?: boolean; }
export const PORTS: Record<NodeKind, { inputs: Port[]; outputs: Port[] }> = {
  text: { inputs: [], outputs: [{ id: 'out', type: 'Text' }] },
  image: { inputs: [], outputs: [{ id: 'out', type: 'ImageRef' }] },
  params: { inputs: [], outputs: [{ id: 'out', type: 'Params' }] },
  generate: { inputs: [{ id: 'prompt', type: 'Text' }, { id: 'params', type: 'Params' }, { id: 'reference', type: 'ImageRef', multiple: true }, { id: 'origin', type: 'ImageRef' }], outputs: [{ id: 'out', type: 'ImageRef' }] },
  edit: { inputs: [{ id: 'prompt', type: 'Text' }, { id: 'image', type: 'ImageRef', required: true }, { id: 'params', type: 'Params' }], outputs: [{ id: 'out', type: 'ImageRef' }] },
  output: { inputs: [{ id: 'image', type: 'ImageRef', required: true }], outputs: [] },
};

let idSequence = 0;
function newId(prefix: string) {
  return `${prefix}_${Date.now().toString(36)}_${(++idSequence).toString(36)}_${Math.random().toString(36).slice(2, 10)}`;
}

export function createNode(kind: NodeKind, projectId: string, position = { x: 0, y: 0 }): CanvasNode {
  if (!isKind(kind)) throw new Error('不支持的节点类型');
  return {
    id: newId('node'), projectId, kind, name: NODE_LABELS[kind], position: { ...position },
    configVersion: 1, config: kind === 'text' ? { text: '' } : kind === 'params' ? { params: { ...DEFAULT_PARAMS } } : ['image', 'generate', 'edit'].includes(kind) ? { text: '', params: { ...DEFAULT_PARAMS } } : {},
    stale: false,
  };
}

/** A new canvas starts with one image card with its own prompt and parameters. */
export function createImageFirstGraph(projectId: string): Graph {
  const image = createNode('generate', projectId, { x: 260, y: 160 });
  image.name = '新图片';
  return { nodes: [image], edges: [], viewport: { x: 0, y: 0, zoom: 1 } };
}

export function createStarterGraph(projectId: string): Graph {
  const prompt = createNode('text', projectId, { x: 50, y: 40 });
  prompt.name = '柔和的午后光线';
  prompt.config.text = '保留主体摆放与拍摄角度，让背景更干净，光线更柔和。温暖自然的午后光线，克制的色彩，真实的食物质感。';
  const image = createNode('image', projectId, { x: 50, y: 350 });
  const params = createNode('params', projectId, { x: 390, y: 440 });
  const edit = createNode('edit', projectId, { x: 730, y: 230 });
  edit.name = '让光线柔和一点';
  const generate = createNode('generate', projectId, { x: 390, y: 40 });
  generate.name = '探索另一个版本';
  const output = createNode('output', projectId, { x: 1070, y: 230 });
  const edge = (source: CanvasNode, target: CanvasNode, port: string): CanvasEdge => ({ id: newId('edge'), sourceNodeId: source.id, sourcePort: 'out', targetNodeId: target.id, targetPort: port });
  return {
    nodes: [prompt, image, params, generate, edit, output],
    edges: [edge(prompt, edit, 'prompt'), edge(image, edit, 'image'), edge(params, edit, 'params'), edge(prompt, generate, 'prompt'), edge(params, generate, 'params'), edge(edit, output, 'image')],
    viewport: { x: 30, y: 50, zoom: 0.7 },
  };
}

function result(errors: string[]): GraphValidation { return { valid: errors.length === 0, errors }; }
function isKind(kind: unknown): kind is NodeKind { return typeof kind === 'string' && Object.hasOwn(PORTS, kind); }
function plainObject(value: unknown): value is Record<string, unknown> { return !!value && typeof value === 'object' && !Array.isArray(value); }

/** The check can also be used while dragging an edge; it never mutates the graph. */
export function validateConnection(graph: Graph, edge: CanvasEdge): GraphValidation {
  const errors: string[] = [];
  if (!edge || !graph || !Array.isArray(graph.nodes) || !Array.isArray(graph.edges)) return result(['连线或画布格式无效']);
  const source = graph.nodes.find(n => n?.id === edge.sourceNodeId);
  const target = graph.nodes.find(n => n?.id === edge.targetNodeId);
  if (!source || !target) return result(['连线引用的节点不存在']);
  if (source.projectId !== target.projectId) errors.push('不能连接不同项目的节点');
  if (source.id === target.id) errors.push('节点不能连接到自身');
  if (!isKind(source.kind) || !isKind(target.kind)) return result([...errors, '不支持的节点类型']);
  const sourcePort = PORTS[source.kind].outputs.find(p => p.id === edge.sourcePort);
  const targetPort = PORTS[target.kind].inputs.find(p => p.id === edge.targetPort);
  if (!sourcePort) errors.push('源端口不存在或方向错误：请从输出端口连接');
  if (!targetPort) errors.push('目标端口不存在或方向错误：请连接输入端口');
  if (sourcePort && targetPort && sourcePort.type !== targetPort.type) errors.push(`端口类型不兼容：${sourcePort.type} 不能连接 ${targetPort.type}`);
  // Do not ignore an edge with a matching id here. Save validation checks each
  // edge against the graph with that one edge removed, while the drag caller
  // supplies the graph before adding an edge.
  if (graph.edges.some(e => e.sourceNodeId === edge.sourceNodeId && e.sourcePort === edge.sourcePort && e.targetNodeId === edge.targetNodeId && e.targetPort === edge.targetPort)) errors.push('这两个端口已经连接');
  if (!targetPort?.multiple && graph.edges.some(e => e.targetNodeId === edge.targetNodeId && e.targetPort === edge.targetPort)) errors.push('每个输入端口最多允许一条连线');
  if (targetPort?.multiple && graph.edges.filter(e => e.targetNodeId === edge.targetNodeId && e.targetPort === edge.targetPort).length >= 8) errors.push('最多同时使用 8 张参考图');
  if (descendants(graph, target.id).includes(source.id)) errors.push('此连接会形成环路，画布只支持有向无环图');
  return result(errors);
}

export function validateGraph(graph: Graph, assets?: AssetVersion[]): GraphValidation {
  if (!graph || !Array.isArray(graph.nodes) || !Array.isArray(graph.edges)) return result(['画布必须包含节点和连线数组']);
  const errors: string[] = [];
  if (graph.nodes.length > 1000 || graph.edges.length > 3000) return result(['画布超出节点或连线数量限制']);
  const ids = new Set<string>();
  let projectId: string | undefined;
  for (const node of graph.nodes) {
    if (!node || typeof node.id !== 'string' || !node.id || node.id.length > 200 || ids.has(node.id)) { errors.push('节点 ID 为空或重复'); continue; }
    ids.add(node.id);
    if (!node.projectId || typeof node.projectId !== 'string') errors.push('节点缺少项目标识');
    else if (projectId && node.projectId !== projectId) errors.push('节点必须属于同一个项目');
    else projectId = node.projectId;
    if (!isKind(node.kind)) errors.push('不支持的节点类型');
    if (typeof node.name !== 'string' || node.name.length > 200) errors.push('节点名称无效或过长');
    if (!node.position || !Number.isFinite(node.position.x) || !Number.isFinite(node.position.y)) errors.push('节点坐标无效');
    if (!plainObject(node.config)) { errors.push('节点配置无效'); continue; }
    if (node.config.text !== undefined && (typeof node.config.text !== 'string' || node.config.text.length > 32000)) errors.push('提示词无效或超过 32000 字符');
    if (node.config.note !== undefined && (typeof node.config.note !== 'string' || node.config.note.length > 4000)) errors.push('备注无效或过长');
    if (node.config.params !== undefined) errors.push(...validateParams(node.config.params));
    if (node.selectedAssetVersionId !== undefined) {
      if (typeof node.selectedAssetVersionId !== 'string' || !node.selectedAssetVersionId) errors.push('素材版本标识无效');
      else if (assets) {
        const asset = assets.find(a => a?.id === node.selectedAssetVersionId);
        if (!asset) errors.push('选定的素材版本不存在');
        else if (asset.projectId !== node.projectId) errors.push('不能引用其他项目的素材');
      }
    }
  }
  const edgeIds = new Set<string>();
  graph.edges.forEach((edge, index) => {
    if (!edge || typeof edge.id !== 'string' || !edge.id || edge.id.length > 200 || edgeIds.has(edge.id)) { errors.push('连线 ID 为空或重复'); return; }
    edgeIds.add(edge.id);
    errors.push(...validateConnection({ ...graph, edges: graph.edges.filter((_, i) => i !== index) }, edge).errors);
  });
  if (graph.viewport && (!Number.isFinite(graph.viewport.x) || !Number.isFinite(graph.viewport.y) || !Number.isFinite(graph.viewport.zoom) || graph.viewport.zoom <= 0 || graph.viewport.zoom > 10)) errors.push('视口参数无效');
  return result([...new Set(errors)]);
}

/** Reachability uses a visited set, so even untrusted cyclic input terminates. */
export function descendants(graph: Graph, nodeId: string): string[] {
  const found = new Set<string>();
  const pending = [nodeId];
  const children = new Map<string, string[]>();
  for (const edge of graph.edges) {
    if (!edge) continue;
    const targets = children.get(edge.sourceNodeId) ?? [];
    targets.push(edge.targetNodeId);
    children.set(edge.sourceNodeId, targets);
  }
  const visited = new Set([nodeId]);
  while (pending.length) {
    const current = pending.pop()!;
    for (const child of children.get(current) ?? []) {
      if (visited.has(child)) continue;
      visited.add(child); found.add(child); pending.push(child);
    }
  }
  return [...found];
}

function stable(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map(stable).join(',')}]`;
  if (value && typeof value === 'object') return `{${Object.keys(value).sort().filter(k => (value as Record<string, unknown>)[k] !== undefined).map(k => `${JSON.stringify(k)}:${stable((value as Record<string, unknown>)[k])}`).join(',')}}`;
  return JSON.stringify(value) ?? 'null';
}

function semanticConfig(node: CanvasNode): unknown {
  switch (node.kind) {
    case 'text': return { text: node.config.text ?? '' };
    case 'params': return { params: node.config.params ?? DEFAULT_PARAMS };
    case 'image': case 'generate': case 'edit': return { text: node.config.text ?? '', params: node.config.params ?? DEFAULT_PARAMS };
    default: return {};
  }
}
function inputEdges(graph: Graph, nodeId: string): string {
  return stable(graph.edges.filter(e => e.targetNodeId === nodeId).map(e => ({ sourceNodeId: e.sourceNodeId, sourcePort: e.sourcePort, targetPort: e.targetPort })).sort((a, b) => stable(a).localeCompare(stable(b))));
}

/** Derive revisions/staleness on the server; ignore caller-supplied versions. */
export function reconcileGraph(previous: Graph, incoming: Graph): Graph {
  const oldNodes = new Map(previous.nodes.map(n => [n.id, n]));
  const changed = new Set<string>();
  const changedInput = new Set<string>();
  const nodes = incoming.nodes.map(node => {
    const old = oldNodes.get(node.id);
    if (!old) return { ...node, position: { ...node.position }, config: structuredClone(node.config), configVersion: 1, stale: false };
    const configChanged = old.kind !== node.kind || stable(semanticConfig(old)) !== stable(semanticConfig(node));
    const assetChanged = old.selectedAssetVersionId !== node.selectedAssetVersionId;
    const edgesChanged = inputEdges(previous, node.id) !== inputEdges(incoming, node.id);
    if (configChanged || assetChanged || edgesChanged) changed.add(node.id);
    if (configChanged || edgesChanged) changedInput.add(node.id);
    return {
      ...node, position: { ...node.position }, config: structuredClone(node.config),
      configVersion: old.configVersion + (configChanged || assetChanged || edgesChanged ? 1 : 0),
      stale: old.stale,
    };
  });
  const graph = { ...incoming, nodes, edges: incoming.edges.map(e => ({ ...e })), viewport: incoming.viewport ? { ...incoming.viewport } : undefined };
  const stale = new Set<string>();
  for (const id of changed) for (const descendant of descendants(graph, id)) stale.add(descendant);
  for (const id of changedInput) stale.add(id);
  return { ...graph, nodes: nodes.map(node => ({ ...node, stale: node.stale || (stale.has(node.id) && ['image', 'generate', 'edit', 'output'].includes(node.kind)) })) };
}

export function getNodeOutputAsset(graph: Graph, nodeId: string): string | undefined {
  const node = graph.nodes.find(n => n.id === nodeId);
  if (!node) return undefined;
  if (node.kind === 'image' || node.kind === 'generate' || node.kind === 'edit') return node.selectedAssetVersionId;
  if (node.kind !== 'output') return undefined;
  const edge = graph.edges.find(e => e.targetNodeId === node.id && e.targetPort === 'image');
  // Only image-producing nodes can feed an output; do not recurse on invalid input.
  const source = edge && graph.nodes.find(n => n?.id === edge.sourceNodeId);
  return source && ['image', 'generate', 'edit'].includes(source.kind) ? source.selectedAssetVersionId : undefined;
}

function validateParams(params: unknown): string[] {
  if (!plainObject(params)) return ['生成参数格式无效'];
  const errors: string[] = [];
  errors.push(...validateImageSize(params.size));
  if (!['low', 'medium', 'high'].includes(params.quality as string)) errors.push('不支持的图像质量');
  if (!Number.isInteger(params.n) || (params.n as number) < 1 || (params.n as number) > 4) errors.push('每次运行的张数必须为 1–4 张');
  return errors;
}

interface InputResolutionContext {
  nodeById: Map<string, CanvasNode>;
  incoming: Map<string, CanvasEdge[]>;
  assetById: Map<string, AssetVersion>;
}

function inputResolutionContext(graph: Graph, assets: AssetVersion[]): InputResolutionContext {
  const nodeById = new Map(graph.nodes.map(node => [node.id, node]));
  const incoming = new Map<string, CanvasEdge[]>();
  for (const edge of graph.edges) {
    const edges = incoming.get(edge.targetNodeId) ?? [];
    edges.push(edge); incoming.set(edge.targetNodeId, edges);
  }
  const assetById = new Map<string, AssetVersion>();
  // Preserve the same first-match behavior as the single-input asset lookup.
  for (const asset of assets) if (asset && !assetById.has(asset.id)) assetById.set(asset.id, asset);
  return { nodeById, incoming, assetById };
}

function validateProjectInputs(project: Project, assets: AssetVersion[]): void {
  const validation = validateGraph(project.graph, assets);
  if (!validation.valid) throw new Error(validation.errors.join('；'));
  if (project.graph.nodes.some(node => node.projectId !== project.id)) throw new Error('节点不属于当前项目');
}

/** Single-target service boundary: always validate the complete graph first. */
export function resolveInput(project: Project, nodeId: string, assets: AssetVersion[], mode: RunMode, model?: string): InputSnapshot {
  validateProjectInputs(project, assets);
  return resolveValidatedInput(project, nodeId, inputResolutionContext(project.graph, assets), mode, model);
}

export interface GraphInputResolution { snapshot?: InputSnapshot; error?: string; }

/**
 * Resolve UI readiness for all executable nodes after one full validation.
 * Callers can memoize this result by graph/assets/mode; changing selection alone
 * must not repeat a full-graph validation for every rendered node card.
 * There is deliberately no externally supplied "skip validation" option.
 */
export function resolveGraphInputs(project: Project, assets: AssetVersion[], mode: RunMode, model?: string): Record<string, GraphInputResolution> {
  const resolved: Record<string, GraphInputResolution> = Object.create(null);
  const executable = Array.isArray(project.graph?.nodes)
    ? project.graph.nodes.filter(node => node && typeof node.id === 'string' && (node.kind === 'image' || node.kind === 'generate' || node.kind === 'edit'))
    : [];
  try {
    validateProjectInputs(project, assets);
  } catch (error) {
    const message = error instanceof Error ? error.message : '画布输入无效';
    for (const node of executable) resolved[node.id] = { error: message };
    return resolved;
  }
  const context = inputResolutionContext(project.graph, assets);
  for (const node of executable) {
    try { resolved[node.id] = { snapshot: resolveValidatedInput(project, node.id, context, mode, model) }; }
    catch (error) { resolved[node.id] = { error: error instanceof Error ? error.message : '节点输入无效' }; }
  }
  return resolved;
}

/** Only called after validateProjectInputs succeeds in the public functions. */
function resolveValidatedInput(project: Project, nodeId: string, context: InputResolutionContext, mode: RunMode, model?: string): InputSnapshot {
  const graph = project.graph;
  const node = context.nodeById.get(nodeId);
  if (!node || (node.kind !== 'image' && node.kind !== 'generate' && node.kind !== 'edit')) throw new Error('只有图片、生成或编辑节点可以运行');
  const source = (port: string) => {
    const edge = context.incoming.get(node.id)?.find(edge => edge.targetPort === port);
    return edge ? context.nodeById.get(edge.sourceNodeId) : undefined;
  };
  const promptEdge = context.incoming.get(node.id)?.find(edge => edge.targetPort === 'prompt');
  const promptNode = source('prompt');
  // An explicit connection owns this input, including when it is empty. Never
  // silently switch an old workflow to a different inline prompt.
  const prompt = promptEdge ? promptNode?.config.text : node.config.text;
  if (!prompt?.trim()) throw new Error(promptEdge ? '请为连接的提示词节点填写非空提示词，或断开连线后独立编辑' : '请在图片侧栏填写非空提示词');
  const paramsNode = source('params');
  const params: ImageParams = structuredClone(paramsNode?.config.params ?? node.config.params ?? DEFAULT_PARAMS);
  const errors = validateParams(params);
  if (errors.length) throw new Error(errors.join('；'));
  if (mode === 'codex' && params.n !== 1) throw new Error('Codex 原生模式每次仅支持 1 张');
  // A plain image edits its own explicitly selected version. This is a frozen
  // run input, not a graph self-edge, and generated results are never adopted here.
  const imageNode = node.kind === 'image' ? node : source('image');
  let asset: AssetVersion | undefined;
  if (node.kind === 'edit' || node.kind === 'image') {
    if (!imageNode) throw new Error('编辑节点需要连接一张主图');
    // A validated ImageRef edge can only start at image/generate/edit.out.
    const assetId = imageNode.selectedAssetVersionId;
    if (!assetId) throw new Error(node.kind === 'image' ? '请先导入图片，或在版本历史中采用一张图片' : '上游尚未选定图片版本，请先生成或导入图片并明确采用一个版本');
    asset = context.assetById.get(assetId);
    if (!asset || asset.projectId !== project.id) throw new Error('主图版本不存在或不属于当前项目');
    if (!/^[a-f0-9]{64}$/i.test(asset.sha256) || !['image/png', 'image/jpeg', 'image/webp'].includes(asset.mime) || !Number.isInteger(asset.width) || !Number.isInteger(asset.height) || !Number.isInteger(asset.byteSize) || asset.width <= 0 || asset.height <= 0 || asset.width * asset.height > 40_000_000 || asset.byteSize <= 0 || asset.byteSize > 20 * 1024 * 1024) throw new Error('主图素材尚未就绪或校验信息无效');
  }
  const references = (context.incoming.get(node.id) ?? []).filter(edge => edge.targetPort === 'reference').map(edge => {
    const sourceNode = context.nodeById.get(edge.sourceNodeId);
    const ref = sourceNode?.selectedAssetVersionId ? context.assetById.get(sourceNode.selectedAssetVersionId) : undefined;
    if (!ref || ref.projectId !== project.id) throw new Error('参考图片尚未采用有效版本');
    if (!/^[a-f0-9]{64}$/i.test(ref.sha256) || !['image/png','image/jpeg','image/webp'].includes(ref.mime) || !Number.isInteger(ref.width) || !Number.isInteger(ref.height) || ref.width <= 0 || ref.height <= 0 || ref.width * ref.height > 40_000_000 || !Number.isInteger(ref.byteSize) || ref.byteSize <= 0 || ref.byteSize > 20 * 1024 * 1024) throw new Error('参考图片尚未就绪或校验信息无效');
    return ref;
  });
  if (references.length) asset = references[0];
  // Every stale generated ancestor is reported, including through fixed images.
  // The caller must ask whether to continue with this explicitly selected old image.
  const ancestors = new Set<string>();
  const pending = [node.id];
  while (pending.length) {
    const current = pending.pop()!;
    for (const edge of context.incoming.get(current) ?? []) {
      if (ancestors.has(edge.sourceNodeId)) continue;
      ancestors.add(edge.sourceNodeId); pending.push(edge.sourceNodeId);
    }
  }
  const upstreamStale = graph.nodes.filter(node => ancestors.has(node.id) && node.stale && (node.kind === 'image' || node.kind === 'generate' || node.kind === 'edit')).map(node => node.id).sort();
  return {
    nodeId: node.id, nodeKind: node.kind === 'image' || references.length ? 'edit' : node.kind, configVersion: node.configVersion,
    prompt, params,
    ...(asset ? { sourceAssetVersionId: asset.id, sourceAssetHash: asset.sha256 } : {}),
    ...(references.length ? { sourceAssetVersionIds: references.map(ref => ref.id), sourceAssetHashes: references.map(ref => ref.sha256) } : {}),
    upstreamStale, model: model ?? (mode === 'api' ? 'gpt-image-1' : mode === 'mock' ? 'canvepp-mock-v1' : mode === 'codex' ? 'codex-native-imagegen' : 'manual'),
    provider: mode === 'api' ? 'openai' : mode === 'mock' ? 'mock' : mode === 'codex' ? 'codex' : 'manual',
  };
}
