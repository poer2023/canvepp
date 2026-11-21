export type NodeKind = 'text' | 'image' | 'params' | 'generate' | 'edit' | 'output';
export type PortType = 'Text' | 'ImageRef' | 'Params';
export type RunMode = 'mock' | 'manual' | 'api' | 'codex';
export type RunStatus = 'queued' | 'submitting' | 'running' | 'awaiting_host' | 'awaiting_import' | 'succeeded' | 'partial' | 'failed' | 'cancelled' | 'unknown';
export type ImageSize = `${number}x${number}`;
export type ApiImageSize = '1024x1024' | '1536x1024' | '1024x1536';
export interface ImageParams { size: ImageSize; quality: 'low' | 'medium' | 'high'; n: number; }
export interface NodeConfig { text?: string; params?: ImageParams; note?: string; }
export interface CanvasNode { id: string; projectId: string; kind: NodeKind; name: string; position: {x:number;y:number}; configVersion: number; config: NodeConfig; selectedAssetVersionId?: string; stale: boolean; }
export interface CanvasEdge { id:string; sourceNodeId:string; sourcePort:string; targetNodeId:string; targetPort:string; }
export interface Graph { nodes: CanvasNode[]; edges: CanvasEdge[]; viewport?: {x:number;y:number;zoom:number}; }
export interface Project { id:string; name:string; schemaVersion:number; graphRevision:number; layoutRevision:number; createdAt:string; updatedAt:string; graph:Graph; }
export interface AssetVersion { id:string; projectId:string; assetId:string; sha256:string; mime:string; width:number; height:number; byteSize:number; createdAt:string; name:string; parentVersionIds:string[]; runId?:string; nodeId?:string; source:'import'|'mock'|'api'|'manual'|'codex'; favorite?:boolean; assessment?:'pass'|'continue'|'redo'; }
export interface InputSnapshot { nodeId:string; nodeKind:'generate'|'edit'; configVersion:number; prompt:string; params:ImageParams; sourceAssetVersionId?:string; sourceAssetHash?:string; sourceAssetVersionIds?:string[]; sourceAssetHashes?:string[]; upstreamStale:string[]; model:string; provider:string; providerOrganization?:string; providerProject?:string; }
export interface RunPreview { id:string; projectId:string; nodeId:string; graphRevision:number; mode:RunMode; snapshot:InputSnapshot; snapshotHash:string; estimatedCost:number; currency:'USD'; warnings:string[]; createdAt:string; expiresAt:string; }
export interface RunEvent { eventId:string; runId:string; sequence:number; state:RunStatus; occurredAt:string; assetVersionIds?:string[]; errorClass?:string; }
export interface Run { historyOnly?:true; id:string; jobId:string; projectId:string; nodeId:string; mode:RunMode; status:RunStatus; inputSnapshot:InputSnapshot; fingerprint:string; createdAt:string; updatedAt:string; assetVersionIds:string[]; providerRequestId?:string; errorClass?:string; error?:string; lateResult?:boolean; cancelRequested?:boolean; draftChanged?:boolean; usage?:Record<string,unknown>; events:RunEvent[]; }
export interface Job { id:string; runId:string; attempt:number; state:RunStatus; providerRequestId?:string; errorClass?:string; createdAt:string; updatedAt:string; }
export interface BudgetSettings { apiEnabled:boolean; organization:string; project:string; creditsConfirmed:boolean; creditsExpireAt:string; availableCredits:number; paidBalanceAcknowledged:boolean; autoRechargeAcknowledged:boolean; perRunLimit:number; monthlyLimit:number; pricePerImage:number; priceValidUntil:string; model:string; }
export interface BudgetSummary { settings:BudgetSettings; reserved:number; spent:number; keyConfigured:boolean; }
export interface ProjectDetail { project:Project; assets:AssetVersion[]; runs:Run[]; budget:BudgetSummary; }
export interface TrashedCard { id:string; projectId:string; node:CanvasNode; edges:CanvasEdge[]; assetVersionIds:string[]; displayAssetVersionId?:string; deletedAt:string; legacy?:boolean; }
export interface GraphValidation { valid:boolean; errors:string[]; }
export interface ProviderCapabilities { provider:string; model:string; sizes:ImageParams['size'][]; qualities:ImageParams['quality'][]; maxImages:number; query:boolean; cancel:boolean; }

/** Private host claim credentials are deliberately absent from Run and exports. */
export interface HostClaimResult { run:Run; claimToken:string; canGenerate:boolean; alreadyClaimed:boolean; }
