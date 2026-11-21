import { DEFAULT_PARAMS, getNodeOutputAsset, validateGraph } from '../domain/graph';
import type { AssetVersion, CanvasNode, Graph, ImageParams, NodeConfig, NodeKind } from '../domain/types';
import { getEditorInputs } from './image-editor';

export interface ImageFirstIssue { nodeId?: string; message: string; }
export class ImageFirstConversionError extends Error {
    constructor(public readonly issues: ImageFirstIssue[]) {
        super(`无法安全转换图片画布：${issues.map(issue => `${issue.nodeId ? `${issue.nodeId}：` : ''}${issue.message}`).join('；')}`);
        this.name = 'ImageFirstConversionError';
    }
}

export interface ImageFirstProvenance {
    nodeId: string;
    originalKind: NodeKind;
    kind: 'image' | 'generate';
    /** Includes inactive local drafts, which must not be mistaken for effective inputs. */
    originalConfig: NodeConfig;
    prompt: string;
    params: ImageParams;
    promptSourceNodeId?: string;
    paramsSourceNodeId?: string;
    inputSourceNodeId?: string;
    originalInputAssetVersionId?: string;
    originalVisibleAssetVersionId?: string;
    selectedAssetVersionId?: string;
    usedOriginalInputAsCurrent: boolean;
}

export interface ImageFirstConversion {
    graph: Graph;
    /** Persist this original graph and provenance before saving the converted graph. */
    backup: Graph;
    provenance: ImageFirstProvenance[];
    removedNodeIds: string[];
    retainedAssetVersionIds: string[];
    warnings: ImageFirstIssue[];
}

/**
 * Explicit migration for an authorized legacy/example graph, never an automatic
 * load-time migration. It does not import/delete assets, modify Runs or execute
 * tasks. The caller must durably save backup + provenance before a graph PATCH.
 * Optional assets validate every existing reference against its actual project.
 */
export function convertGraphToImageFirst(graph: Graph, assets?: readonly AssetVersion[]): ImageFirstConversion {
    const validation = validateGraph(graph, assets ? [...assets] : undefined);
    if (!validation.valid) throw new ImageFirstConversionError(validation.errors.map(message => ({ message })));
    const issues: ImageFirstIssue[] = [];
    const removed = graph.nodes.filter(node => node.kind === 'text' || node.kind === 'params');
    const nodeById = new Map(graph.nodes.map(node => [node.id, node]));
    for (const node of removed) {
        const consumed = graph.edges.some(edge => edge.sourceNodeId === node.id && !['text', 'params'].includes(nodeById.get(edge.targetNodeId)!.kind));
        const content = node.kind === 'text' ? !!node.config.text?.length : node.config.params !== undefined;
        if (!consumed && content) issues.push({ nodeId: node.id, message: '独立输入仍有未使用的内容，无法在移除前并入图片' });
        if (node.config.note?.length) issues.push({ nodeId: node.id, message: '输入卡片有独有备注，不能自动丢弃或覆盖图片备注' });
        if (node.selectedAssetVersionId) issues.push({ nodeId: node.id, message: '输入卡片有图片版本引用，不能自动移除' });
        if (node.kind === 'text' && node.config.params !== undefined || node.kind === 'params' && !!node.config.text?.length) {
            issues.push({ nodeId: node.id, message: '输入卡片有额外配置内容，需要先明确保留位置' });
        }
    }
    for (const node of graph.nodes) {
        if (node.kind === 'output' && node.selectedAssetVersionId && node.selectedAssetVersionId !== getNodeOutputAsset(graph, node.id)) {
            issues.push({ nodeId: node.id, message: '输出卡片保存了不同于可见上游图的独立图片引用，不能自动覆盖' });
        }
    }
    if (issues.length) throw new ImageFirstConversionError(issues);

    const provenance: ImageFirstProvenance[] = [];
    const warnings: ImageFirstIssue[] = [];
    const retainedAssetIds = new Set<string>();
    const nodes = graph.nodes.filter(node => node.kind !== 'text' && node.kind !== 'params').map(node => {
        const inputEdge = graph.edges.find(edge => edge.targetNodeId === node.id && edge.targetPort === 'image');
        const inputSource = inputEdge ? nodeById.get(inputEdge.sourceNodeId) : undefined;
        const controlsNode = node.kind === 'output' && inputSource ? inputSource : node;
        const inputs = getEditorInputs(graph, controlsNode);
        const prompt = inputs.prompt;
        const params = structuredClone(inputs.params ?? DEFAULT_PARAMS);
        const visible = getNodeOutputAsset(graph, node.id);
        const originalInput = inputSource ? getNodeOutputAsset(graph, inputSource.id) : undefined;
        // A legacy edit's selected version is its result, not its source. An edit
        // without a selected result instead starts an independent image from its
        // exact selected input. Empty outputs remain empty generation placeholders.
        const selected = node.kind === 'edit' ? visible ?? originalInput : visible;
        const kind: 'image' | 'generate' = selected || node.kind === 'image' ? 'image' : 'generate';
        for (const assetId of [node.selectedAssetVersionId, visible, originalInput, selected]) if (assetId) retainedAssetIds.add(assetId);
        provenance.push({
            nodeId: node.id, originalKind: node.kind, kind, originalConfig: structuredClone(node.config), prompt, params: structuredClone(params),
            promptSourceNodeId: inputs.promptSource?.id, paramsSourceNodeId: inputs.paramsSource?.id,
            inputSourceNodeId: inputSource?.id, originalInputAssetVersionId: originalInput,
            originalVisibleAssetVersionId: visible, selectedAssetVersionId: selected,
            usedOriginalInputAsCurrent: node.kind === 'edit' && !visible && !!originalInput,
        });
        if (node.kind === 'edit' && !selected) warnings.push({ nodeId: node.id, message: '未选定结果或可用原图，已保留为待创作图片' });
        const next: CanvasNode = { ...structuredClone(node), kind, config: { ...structuredClone(node.config), text: prompt, params }, selectedAssetVersionId: selected };
        return next;
    });
    const converted: Graph = { ...structuredClone(graph), nodes, edges: [] };
    const convertedValidation = validateGraph(converted, assets ? [...assets] : undefined);
    if (!convertedValidation.valid) throw new ImageFirstConversionError(convertedValidation.errors.map(message => ({ message })));
    return { graph: converted, backup: structuredClone(graph), provenance, removedNodeIds: removed.map(node => node.id), retainedAssetVersionIds: [...retainedAssetIds], warnings };
}
