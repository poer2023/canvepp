'use client';
import React, { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, useSyncExternalStore } from 'react';
import { ReactFlow, ReactFlowProvider, Background, BackgroundVariant, MiniMap, Handle, Position, MarkerType, useReactFlow, useUpdateNodeInternals, type ReactFlowProps, type NodeProps, type Node as FlowNode, type Connection, type NodeChange, type EdgeChange } from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import { ArrowDownToLine, ArrowLeft, ArrowRight, ArrowUpRight, Check, CheckCheck, ChevronDown, ChevronLeft, ChevronRight, ChevronUp, CircleHelp, Clock3, Copy, Download, Expand, FileImage, FilePlus2, FolderOpen, GitBranch, Grip, History, ImagePlus, Images, LayoutGrid, Link2, Loader2, Maximize2, MousePointer2, Play, Plus, Redo2, Search, ShieldCheck, SlidersHorizontal, Sparkles, Square, Star, Trash2, Type, Undo2, Upload, WandSparkles, X, ZoomIn, ZoomOut, AlertCircle, Circle, ExternalLink } from 'lucide-react';
import type { AssetVersion, TrashedCard, CanvasEdge, CanvasNode, Graph, ImageParams, NodeKind, Project, ProjectDetail, Run, RunMode, RunPreview, RunStatus } from '@/domain/types';
import { consumeCodexHostToken, getCodexHostToken, readCodexHostContext, type CodexHostContext, type CodexDispatchReceipt } from '@/lib/codex-host';
import { sameDraft, sharedGraph, readDraftBackup, writeDraftBackup, readViewport, writeViewport, type Draft } from '@/lib/project-draft';
import { mergeProjectDraft, resolveProjectDraftConflicts, type MergeConflict, type MergeResolutions } from '@/lib/project-merge';
import { copySelection, pasteSelection, deleteSelection, restoreGraphSnapshot, type CanvasClipboard } from '@/lib/canvas-editing';
import { createCanvasPositionStore, type CanvasPositionStore } from '@/lib/canvas-positions';
import { ImageViewerSurface } from '@/components/image-viewer-surface';
import { ImageControls, API_IMAGE_SIZES } from '@/components/image-controls';
import { MultiReferenceSettings } from '@/components/generation-settings';
import { generationGroup, snapshotSources, type GenerationGroupPreview, type GenerationGroupResult } from '@/domain/generation-group';
import { CanvasContextMenu, type CanvasMenuAction } from '@/components/canvas-context-menu';
import { TrashPanel } from '@/components/trash-panel';
import { RunHistory } from '@/components/run-history';
import { decodeCanvasClipboard, imageFiles, transferImageUrls, fileFromImageUrl, readCanvasClipboard, writeCanvasClipboard } from '@/lib/canvas-transfer';
import { ImageLibrary } from '@/components/image-library';
import { localLibrary, type LibraryImage, type LibraryPage } from '@/domain/image-library';
import { imageSizeRatio } from '@/domain/image-size';
import { findOpenPosition, getEditorInputs, isImageCreator, makeInputIndependent } from '@/lib/image-editor';
import { createNode, DEFAULT_PARAMS, descendants, getNodeOutputAsset, NODE_LABELS, PORTS, reconcileGraph, resolveGraphInputs, validateConnection } from '@/domain/graph';
const ICONS = { text: Type, image: ImagePlus, params: SlidersHorizontal, generate: Sparkles, edit: WandSparkles, output: ArrowDownToLine };
const UI_NODE_LABELS:Record<NodeKind,string>={...NODE_LABELS,image:'图片',generate:'图片'};
const PORT_LABEL: Record<string, string> = { prompt: '提示词', image: '主图', params: '参数', reference: '参考图', origin: '来源', out: '输出' };
const MODE_NAMES: Record<RunMode, string> = { codex:'Codex 原生', mock: '模拟模式', manual: '手动模式', api: 'Images API' };
const STATUS_NAMES: Record<RunStatus, string> = { queued: '排队中', submitting: '提交中', running: '生成中', awaiting_host:'等待 Codex', awaiting_import: '等待回填', succeeded: '已完成', partial: '部分完成', failed: '失败', cancelled: '已取消', unknown: '待对账' };
const ACTIVE: RunStatus[] = ['queued', 'submitting', 'running'];
const PENDING:RunStatus[]=['awaiting_host',...ACTIVE];
function runStatusName(run: Run) {
    if (run.mode === 'codex') {
        if (run.status === 'unknown') return '状态待确认';
        if (run.status === 'awaiting_host' && run.usage?.canvasExecution) return '排队中';
        if (run.status === 'awaiting_host' && hostDispatchStatus(run) === 'unknown') return '交接待确认';
    }
    return STATUS_NAMES[run.status];
}
function hostDispatchStatus(run:Run,local?:CodexDispatchReceipt['status']):CodexDispatchReceipt['status']|undefined {
    const saved=run.usage?.hostDispatch;
    const status=saved&&typeof saved==='object'&&'status' in saved?saved.status:undefined;
    if(status==='queued')return 'queued';
    if(local)return local;
    return status==='unknown'||status==='sending'?'unknown':undefined;
}
const EMPTY_ASSETS:AssetVersion[]=[];const EMPTY_RUNS:Run[]=[];
const clone = <T,>(value: T): T => structuredClone(value);
const id = () => crypto.randomUUID();
const equalIds = (a:string[],b:string[]) => a.length===b.length&&a.every(value=>b.includes(value));
const MERGE_PENDING = '请先选择同时修改的内容';
const SAVE_RETRY = '正在合并新的修改';
interface PendingMerge { base: Project; local: Draft; remote: Project; conflicts: MergeConflict[]; resolutions?: MergeResolutions; }
const assetUrl = (asset?: AssetVersion | string) => `/api/assets/${typeof asset === 'string' ? asset : asset?.id}`;
const thumbnailUrl = (asset: AssetVersion | string) => `${assetUrl(asset)}?thumbnail=1`;
const fmtTime = (date: string) => new Date(date).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' });
let csrfToken = '';
let sessionPromise: Promise<void> | undefined;
function refreshLocalSession() {
    sessionPromise ??= fetch('/api/session', {cache:'no-store'})
        .then(async response => {
            if (!response.ok) throw new Error('无法连接本地画布服务');
            csrfToken = (await response.json() as {csrfToken:string}).csrfToken;
        }).finally(() => { sessionPromise = undefined; });
    return sessionPromise;
}
class RequestError extends Error {
    status: number;
    constructor(message: string, status: number) { super(message); this.status = status; }
}
async function api<T>(url: string, method = 'GET', payload?: unknown, hostToken?: string): Promise<T> {
    const headers: Record<string, string> = hostToken ? { 'X-Canvepp-Host': hostToken } : {};
    const write = method !== 'GET';
    if (write && !csrfToken) await refreshLocalSession();
    let body: BodyInit | undefined;
    if (payload instanceof FormData) {
        if (!payload.has('requestId'))
            payload.set('requestId', id());
        body = payload;
    }
    else if (payload !== undefined) {
        headers['Content-Type'] = 'application/json';
        body = JSON.stringify({ ...payload as object, requestId: id() });
    }
    const send = () => {
        if (write) headers['X-CSRF-Token'] = csrfToken;
        return fetch(`/api${url}`, {method, headers, body, cache:'no-store'});
    };
    let response = await send();
    if (response.status === 401 || response.status === 403) {
        const reason = await response.clone().json().catch(() => ({}));
        // Both codes originate before request dispatch. Keep the exact original body and confirmation key.
        if (reason.errorClass === 'session_expired' || reason.errorClass === 'csrf_mismatch') {
            await refreshLocalSession();
            response = await send();
        }
    }
    if (!response.ok) {
        const reason = await response.json().catch(() => ({}));
        throw new RequestError(reason.error || '操作失败，请重试', response.status);
    }
    return response.json();
}
const fetchLibraryPage = (query: string) => api<LibraryPage>(`/image-library?${query}`);
function downloadText(name: string, text: string, mime = 'text/plain') { const a = document.createElement('a'); a.href = URL.createObjectURL(new Blob([text], { type: mime })); a.download = name; a.click(); setTimeout(() => URL.revokeObjectURL(a.href), 2000); }
function downloadAsset(asset: AssetVersion) { const a = document.createElement('a'); a.href = assetUrl(asset); a.download = /\.(png|jpe?g|webp)$/i.test(asset.name)?asset.name:`${asset.name || asset.id}.${asset.mime.split('/')[1] || 'png'}`; a.click(); }
type CardData = {
    item: CanvasNode;
    asset?: AssetVersion;
    run?: Run;
    versionCount: number;
    showPorts: boolean;
    inputPortIds: string[];
    outputPortIds: string[];
    highResolution: boolean;
    resolvedCount: number;
    onRun: (id: string) => void;
    onView: (id: string) => void;
    onUpload: (id: string) => void;
    onSelect: (id: string) => void;
    onHistory: (id: string) => void;
    runReason?: string;
    onExport: (id: string) => void;
};
type CardNode = FlowNode<CardData, 'canvas'>;
type CanvasSelection = { nodeIds: string[]; edgeIds: string[]; activeId?: string };
type HistoryEntry = { graph: Graph; selection: CanvasSelection };
type Positions = Record<string, { x: number; y: number }>;
function NodeCard({ data, selected }: NodeProps<CardNode>) {
    const { item, asset, run } = data;
    const Icon = ICONS[item.kind];
    const ports = {
        inputs: PORTS[item.kind].inputs.filter(port => data.inputPortIds.includes(port.id)),
        outputs: PORTS[item.kind].outputs.filter(port => data.outputPortIds.includes(port.id)),
    };
    const imageSurface = ['image', 'generate', 'edit', 'output'].includes(item.kind);
    const mediaHeight = asset && asset.width > 0 ? Math.max(112, Math.min(360, 246 * asset.height / asset.width)) : 220;
    const updateNodeInternals = useUpdateNodeInternals();
    const geometry = `horizontal:${data.showPorts}:${data.inputPortIds.join(',')}:${data.outputPortIds.join(',')}:${imageSurface ? mediaHeight : ''}`;
    const previousGeometry = useRef(geometry);
    useEffect(() => {
        if (previousGeometry.current === geometry) return;
        previousGeometry.current = geometry;
        updateNodeInternals(item.id);
    }, [geometry, item.id, updateNodeInternals]);
    const busy = run && PENDING.includes(run.status);
    const newResult = !!run?.assetVersionIds.length && !run.assetVersionIds.includes(item.selectedAssetVersionId || '');
    const placeholder = item.kind === 'image' ? '导入图片，开始编辑' : item.kind === 'output' ? '连接图片，收集作品' : data.versionCount > 0 ? '已有结果，待采用' : '在右侧描述想要的画面';
    if (imageSurface) return <div className={`canvas-node image-surface kind-${item.kind} ${selected ? 'selected' : ''}`} style={{ height: mediaHeight }} tabIndex={0} role="group" aria-label={`${item.name}，${asset ? '点击编辑图片' : placeholder}`}>
        <Handle type="source" position={Position.Right} id="generation-source" className="generation-port" isConnectable={false}/>
        <Handle type="target" position={Position.Left} id="generation-target" className="generation-port" isConnectable={false}/>
        {asset ? <img className="surface-image" src={data.highResolution ? assetUrl(asset) : thumbnailUrl(asset)} alt={asset.name || '图片'} draggable={false} loading="lazy" decoding="async"/> : <div className="surface-placeholder" role="button" tabIndex={0} aria-label={placeholder} onKeyDown={event=>{if(event.key==='Enter'||event.key===' '){event.preventDefault();event.stopPropagation();data.onSelect(item.id);}}}><Icon size={32} strokeWidth={1.1}/></div>}
        <div className="surface-chrome surface-top">
            <div className="surface-title"><span>{item.name}</span>{data.versionCount > 1 && <small>{data.versionCount} 个版本</small>}</div>
            <div className="surface-top-actions">
                {asset && <button className="surface-icon-button nodrag" title="查看大图" aria-label="查看大图" onClick={event => { event.stopPropagation(); data.onView(asset.id); }}><Expand size={15}/></button>}
                <button className="surface-icon-button nodrag" title="版本历史" aria-label="版本历史" onClick={event => { event.stopPropagation(); data.onHistory(item.id); }}><History size={15}/></button>
            </div>
        </div>
        <div className="surface-chrome surface-bottom">
            {!asset && <span className="surface-hint">{placeholder}</span>}
            {newResult && <button className="surface-result nodrag" onClick={event => { event.stopPropagation(); data.onHistory(item.id); }}><Sparkles size={12}/>有新结果，选择采用<ArrowUpRight size={12}/></button>}
            {run && ['failed', 'unknown', 'partial'].includes(run.status) && <div className="surface-status" title={run.error}><AlertCircle size={12}/>{runStatusName(run)}{run.status === 'unknown' ? run.mode === 'codex' ? ' · 查看生成历史' : ' · 请先对账' : ''}</div>}
            {run?.draftChanged && <span className="surface-status"><History size={12}/>任务完成后输入已变更</span>}
            <div className="surface-action-row">
                {busy && <span className="surface-status"><Loader2 size={12} className="spin"/>{STATUS_NAMES[run.status]}</span>}
                {item.kind === 'image' && <button className="surface-icon-button nodrag" title="导入图片版本" aria-label="导入图片版本" onClick={event => { event.stopPropagation(); data.onUpload(item.id); }}><Upload size={15}/></button>}
                {asset && <button className="surface-icon-button nodrag" title="导出图片" aria-label="导出图片" onClick={event => { event.stopPropagation(); data.onExport(asset.id); }}><Download size={15}/></button>}
            </div>
        </div>
        {data.showPorts && <div className="surface-ports">{ports.inputs.map((port, index) => <div className="surface-input-port" style={{ top: `${(index + 1) * 100 / (ports.inputs.length + 1)}%` }} key={port.id}><Handle type="target" position={Position.Left} id={port.id} className={`typed-port port-${port.type}`} style={{ top: '50%' }}/><span>{PORT_LABEL[port.id]}{port.required ? '' : ' · 可选'}</span></div>)}{ports.outputs.map(port => <div className="surface-output-port" key={port.id}><span>图片输出</span><Handle type="source" position={Position.Right} id={port.id} className={`typed-port port-${port.type}`} style={{ top: '50%' }}/></div>)}</div>}
    </div>;
    return <div className={`canvas-node kind-${item.kind} ${selected ? 'selected' : ''}`}>
        <div className="node-heading"><span className={`node-kind-icon ${item.kind}`}><Icon size={14}/></span><span className="node-name">{item.name}</span><span className="node-kind-name">{UI_NODE_LABELS[item.kind]}</span></div>
        {item.kind === 'text' && <div className="node-text-preview"><span className="small-eyebrow">PROMPT</span><p>{item.config.text || '选择节点，写下你的创作想法…'}</p><div className="node-text-count">{item.config.text?.length || 0} 字符 <Type size={12}/></div></div>}
        {item.kind === 'params' && <div className="node-params"><div><span>画幅</span><strong>{imageSizeRatio(item.config.params?.size) || '自定义'}</strong></div><div><span>质量</span><strong>{{ low: '标准', medium: '精细', high: '高品质' }[item.config.params?.quality || 'medium']}</strong></div><div><span>生成数量</span><strong>{item.config.params?.n || 1} 张</strong></div></div>}
        {data.showPorts && <div className="node-ports">{ports.inputs.map(port => <div className="port-row" key={port.id}><Handle type="target" position={Position.Left} id={port.id} className={`typed-port port-${port.type}`} style={{ top: '50%' }}/><span>{PORT_LABEL[port.id] || port.id}</span><span className="port-type">{port.type === 'ImageRef' ? '图像' : port.type === 'Text' ? '文本' : '参数'}{!port.required ? ' · 可选' : ''}</span></div>)}{ports.outputs.map(port => <div className="port-row port-output" key={port.id}><span>{port.type === 'Text' ? '文本' : port.type === 'ImageRef' ? '图像' : '参数'}</span><Handle type="source" position={Position.Right} id={port.id} className={`typed-port port-${port.type}`} style={{ top: '50%' }}/></div>)}</div>}
    </div>;
}
const nodeTypes = { canvas: React.memo(NodeCard, (before, after) => before.data === after.data && before.selected === after.selected) };
const FIT_VIEW_OPTIONS = { padding: .18 };
const PAN_BUTTONS = [1, 2];
const MULTI_SELECT_KEYS = ['Meta', 'Control'];
const FLOW_PRO_OPTIONS = { hideAttribution: false };
const DEFAULT_VIEWPORT = { x: 0, y: 0, zoom: .8 };
const EMPTY_CARDS: CardNode[] = [];
const minimapColor = () => '#cacaca';
const CanvasFlowSurface = React.memo(function CanvasFlowSurface({ positionStore, showMiniMap, nodes = EMPTY_CARDS, ...props }: ReactFlowProps<CardNode> & { positionStore: CanvasPositionStore; showMiniMap: boolean }) {
    const positions = useSyncExternalStore(positionStore.subscribe, positionStore.getSnapshot, positionStore.getServerSnapshot);
    const previousNodes = useRef(new Map<string, CardNode>());
    const displayNodes = useMemo(() => {
        const next = new Map<string, CardNode>();
        const result = nodes.map(node => {
            const point = positions[node.id];
            const old = previousNodes.current.get(node.id);
            // Stable untouched nodes let React Flow skip rebuilding their geometry.
            const shown = !point ? node : old && old.data === node.data && old.selected === node.selected && old.measured === node.measured && old.dragging && old.position.x === point.x && old.position.y === point.y ? old : { ...node, position: point, dragging: true };
            next.set(node.id, shown);
            return shown;
        });
        previousNodes.current = next;
        return result;
    }, [nodes, positions]);
    return <ReactFlow<CardNode> {...props} nodes={displayNodes}>
        <Background variant={BackgroundVariant.Dots} gap={22} size={1.15} color="#d0d0d0"/>
        {showMiniMap && <MiniMap position="bottom-right" nodeColor={minimapColor} maskColor="rgba(240,240,240,.7)"/>}
    </ReactFlow>;
});
export default function CanvasWorkspace() { return <ReactFlowProvider><Workspace /></ReactFlowProvider>; }
function Workspace() {
    const flow = useReactFlow<CardNode>();
    const [detail, setDetail] = useState<ProjectDetail | null>(null);
    const [projects, setProjects] = useState<Project[]>([]);
    const [graph, setGraph] = useState<Graph>({ nodes: [], edges: [] });
    const [activeId, setActiveId] = useState<string>();
    const [selectedIds, setSelectedIds] = useState<string[]>([]);
    // Controlled React Flow nodes must retain ResizeObserver dimensions across
    // domain/poll updates; otherwise adoptUserNodes resets geometry and hides them.
    const [nodeMeasurements, setNodeMeasurements] = useState<Record<string, {width:number;height:number}>>({});
    const [selectedEdges, setSelectedEdges] = useState<string[]>([]);
    const [loading, setLoading] = useState(true);
    const [saveState, setSaveState] = useState<'saved' | 'saving' | 'unsaved' | 'conflict' | 'error'>('saved');
    const [toast, setToast] = useState('');
    const [mergeReview, setMergeReview] = useState<PendingMerge>();
    const pendingMerge = useRef<PendingMerge | undefined>(undefined);
    const baseProject = useRef<Project | null>(null);
    const [mode, setMode] = useState<RunMode>('codex');
    const [sizeDraftErrors, setSizeDraftErrors] = useState<Record<string, string | undefined>>({});
    const sizeDraftErrorsRef = useRef<Record<string, string | undefined>>({});
    const setSizeValidity = useCallback((nodeId: string, error?: string) => {
        if (sizeDraftErrorsRef.current[nodeId] === error) return;
        sizeDraftErrorsRef.current = { ...sizeDraftErrorsRef.current, [nodeId]: error };
        setSizeDraftErrors(sizeDraftErrorsRef.current);
    }, []);
    const [hostContext,setHostContext]=useState<CodexHostContext>({connected:false});
    const [hostDispatches,setHostDispatches]=useState<Record<string,CodexDispatchReceipt['status']>>({});
    const [hostDispatchBusy,setHostDispatchBusy]=useState<string>();
    const hostDispatchInFlight=useRef(new Set<string>());
    const [metadataPolicy, setMetadataPolicy] = useState<'strip' | 'preserve'>('strip');
    const [panelOpen, setPanelOpen] = useState(true);
    const [historyOpen, setHistoryOpen] = useState(false);
    const [historyView, setHistoryView] = useState<'images' | 'runs'>('runs');
    const [trashOpen, setTrashOpen] = useState(false);
    const [trash, setTrash] = useState<TrashedCard[]>([]);
    const [trashBusy, setTrashBusy] = useState(false);
    const [trashError, setTrashError] = useState('');
    const [trashRefresh, setTrashRefresh] = useState(0);
    const [contextMenu, setContextMenu] = useState<{x:number;y:number;point:{x:number;y:number};nodeIds:string[]}>();
    const [dropActive, setDropActive] = useState(false);
    const [deleteBusy, setDeleteBusy] = useState(false);
    const deleting = useRef(false), importing = useRef(false);
    const [versionsOpen, setVersionsOpen] = useState(false);
    const [libraryPreview, setLibraryPreview] = useState<AssetVersion>();
    const historyOpenRef = useRef(false);
    historyOpenRef.current = historyOpen;
    const openHistory = useCallback(() => { setHistoryOpen(true); setTrashOpen(false); if(window.innerWidth < 1100) setPanelOpen(false); }, []);
    const openInspector = useCallback(() => { setPanelOpen(true); if(window.innerWidth < 1100) {setHistoryOpen(false);setTrashOpen(false);} }, []);
    useEffect(() => {
        const narrow = window.matchMedia('(max-width:1099px)');
        const fitPanels = () => { if(narrow.matches && (historyOpen || trashOpen) && panelOpen) setPanelOpen(false); };
        fitPanels(); narrow.addEventListener('change', fitPanels);
        return () => narrow.removeEventListener('change', fitPanels);
    }, [historyOpen, trashOpen, panelOpen]);
    const openRunHistory = useCallback(() => {openHistory();setHistoryView('runs');}, [openHistory]);
    const openTrash = () => {setTrashOpen(true);setHistoryOpen(false);setTrashRefresh(value=>value+1);if(window.innerWidth<1100)setPanelOpen(false);};
    const [preview, setPreview] = useState<RunPreview>();
    const [groupPreview, setGroupPreview] = useState<GenerationGroupPreview>();
    const [previewBusy, setPreviewBusy] = useState(false);
    const [runBusy, setRunBusy] = useState(false);
    const [acceptStale, setAcceptStale] = useState(false);
    const [viewer, setViewer] = useState<string>();
    const [compareIds, setCompareIds] = useState<string[]>([]);
    const [compareOpen, setCompareOpen] = useState(false);
    const [compareMode, setCompareMode] = useState<'side' | 'slide'>('side');
    const [slider, setSlider] = useState(50);
    const [deleteTarget, setDeleteTarget] = useState<{ nodeIds: string[]; edgeIds: string[] }>();
    const [helpOpen, setHelpOpen] = useState(false);
    const [projectMenu, setProjectMenu] = useState(false);
    const [canvasPickerOpen, setCanvasPickerOpen] = useState(false);
    const [restoreOpen, setRestoreOpen] = useState(false);
    const [projectBusy, setProjectBusy] = useState(false);
    const [attachRun, setAttachRun] = useState<Run>();
    const [attachAssets, setAttachAssets] = useState<string[]>([]);
    const [sourceNote, setSourceNote] = useState('');
    const [attachConfirm, setAttachConfirm] = useState(false);
    const [uploading, setUploading] = useState(false);
    const mutationBusy = useRef(false);
    mutationBusy.current = uploading || projectBusy || runBusy;
    const mutationWaiters = useRef<(() => void)[]>([]);
    useEffect(() => {
        if (!mutationBusy.current) for (const resolve of mutationWaiters.current.splice(0)) resolve();
    }, [uploading, projectBusy, runBusy]);
    const [showRun, setShowRun] = useState<Run>();
    useEffect(()=>{if(showRun)openRunHistory();},[showRun,openRunHistory]);
    const [reconcileOutcome, setReconcileOutcome] = useState<'not_accepted' | 'charged_no_output'>('not_accepted');
    const [reconcileNote, setReconcileNote] = useState('');
    const [reconcileConfirm, setReconcileConfirm] = useState(false);
    const [showMiniMap, setShowMiniMap] = useState(false);
    const [zoom, setZoom] = useState(100);
    const [undoCount, setUndoCount] = useState(0);
    const [redoCount, setRedoCount] = useState(0);
    const fileInput = useRef<HTMLInputElement>(null);
    const packageInput = useRef<HTMLInputElement>(null);
    const uploadTarget = useRef<string | undefined>(undefined);
    const graphRef = useRef(graph);
    const detailRef = useRef(detail);
    const dirty = useRef(false);
    const changeVersion = useRef(0);
    const savePromise = useRef<Promise<Project> | null>(null);
    const saveCurrentRef = useRef<(() => Promise<Project>) | undefined>(undefined);
    const previewHostToken = useRef<string | undefined>(undefined);
    const loadRequest = useRef(0);
    const undoStack = useRef<HistoryEntry[]>([]);
    const redoStack = useRef<HistoryEntry[]>([]);
    const selectionRef = useRef<CanvasSelection>({ nodeIds: [], edgeIds: [] });
    selectionRef.current = { nodeIds: selectedIds, edgeIds: selectedEdges, activeId };
    const clipboard = useRef<CanvasClipboard | undefined>(undefined);
    const pasteIndex = useRef(0);
    const clipboardPending = useRef<CanvasClipboard | undefined>(undefined);
    const clipboardOwned = useRef(false), clipboardLocalOnly = useRef(false);
    useEffect(()=>{
        const release=()=>{clipboardOwned.current=false;};
        // Leaving the canvas app allows another application to replace the clipboard.
        window.addEventListener('blur',release);document.addEventListener('copy',release,true);
        const visibility=()=>{if(document.hidden)release();};document.addEventListener('visibilitychange',visibility);
        return()=>{window.removeEventListener('blur',release);document.removeEventListener('copy',release,true);document.removeEventListener('visibilitychange',visibility);};
    },[]);
    const [positionStore] = useState(createCanvasPositionStore);
    const [dragEndEpoch, setDragEndEpoch] = useState(0);
    const positionsRef = useRef<Positions>({});
    const dragActive = useRef(false);
    const dragWaiters = useRef<(() => void)[]>([]);
    const finishDragRef = useRef<() => void>(() => {});
    const workspaceRef = useRef<HTMLElement>(null);
    const confirmationKey = useRef('');
    const notify = useCallback((message: string) => { if (message !== MERGE_PENDING && message !== SAVE_RETRY) setToast(message); }, []);
    useEffect(() => { if (toast) {
        const t = setTimeout(() => setToast(''), 4500);
        return () => clearTimeout(t);
    } }, [toast]);
    const persistDraft = useCallback(() => {
        const current = detailRef.current, base = baseProject.current;
        if (!current || !base) return;
        const review = pendingMerge.current;
        const ok = writeDraftBackup(review?.base ?? base, review?.local ?? { name: current.project.name, graph: sharedGraph(graphRef.current) });
        if (!ok) notify('浏览器草稿空间不足，请保持页面打开直到保存完成。');
    }, [notify]);
    const restore = useCallback((next: ProjectDetail) => {
        const backup = readDraftBackup(next.project.id);
        const result = backup ? mergeProjectDraft(backup.base, backup.local, next.project) : undefined;
        const viewport = readViewport(next.project.id) ?? next.project.graph.viewport;
        const restoredGraph = { ...(result ? reconcileGraph(next.project.graph, result.merged.graph) : next.project.graph), viewport };
        const restored = { ...next, project: { ...next.project, name: result?.merged.name ?? next.project.name, graph: restoredGraph } };
        baseProject.current = next.project;
        const review = backup && result?.conflicts.length ? { base: backup.base, local: backup.local, remote: next.project, conflicts: result.conflicts } : undefined;
        pendingMerge.current = review; setMergeReview(review);
        detailRef.current = restored; graphRef.current = restoredGraph;
        setDetail(restored); setGraph(restoredGraph);
        dirty.current = !!review || !sameDraft(next.project, restored.project);
        changeVersion.current = 0; setSaveState(review ? 'conflict' : dirty.current ? 'unsaved' : 'saved');
        const initialActive = restoredGraph.nodes.find(n => n.kind === 'edit')?.id || restoredGraph.nodes.find(n => n.kind === 'generate')?.id || restoredGraph.nodes[0]?.id;
        setActiveId(initialActive); setSelectedIds(initialActive ? [initialActive] : []); setSelectedEdges([]);
         setHistoryOpen(false); setTrashOpen(false); setContextMenu(undefined); openInspector(); setPreview(undefined); setGroupPreview(undefined); setShowRun(undefined);  setViewer(undefined); setCompareIds([]); setCompareOpen(false); setAttachRun(undefined); setDeleteTarget(undefined); undoStack.current = []; redoStack.current = []; setUndoCount(0); setRedoCount(0);
        persistDraft();
        setTimeout(() => {
            if (detailRef.current?.project.id !== next.project.id) return;
            if (viewport) flow.setViewport(viewport, { duration: 0 });
            else flow.fitView({ ...FIT_VIEW_OPTIONS, duration: 300 });
        }, 100);
    }, [flow, persistDraft, openInspector]);
    const loadProject = useCallback(async (projectId: string) => { const request = ++loadRequest.current; setLoading(true); try {
        const loaded = await api<ProjectDetail>(`/projects/${projectId}`);
        if (request === loadRequest.current) restore(loaded);
    }
    catch (e) {
        if (request === loadRequest.current) notify((e as Error).message);
    }
    finally {
        if (request === loadRequest.current) setLoading(false);
    } }, [notify, restore]);
    useEffect(() => {
        let live = true, sessionReady = false, hostReadVersion = 0;
        let useLegacyLink = new URLSearchParams(window.location.hash.slice(1)).has('canveppHost');
        const refreshHostContext = async () => {
            const version = ++hostReadVersion;
            const token = consumeCodexHostToken();
            setHostContext({ connected: false });
            setPreview(undefined); setGroupPreview(undefined);
            if (!sessionReady) return;
            setLoading(true);
            try {
                if (mutationBusy.current) await new Promise<void>(resolve => mutationWaiters.current.push(resolve));
                if (!live || version !== hostReadVersion) return;
                // Finish the old canvas first; even an offline/conflicting draft
                // remains saved under its own project ID before switching chats.
                finishDragRef.current();
                persistDraft();
                if (detailRef.current && saveCurrentRef.current) {
                    try { await saveCurrentRef.current(); }
                    catch { persistDraft(); notify('上一张画布的草稿已保留，重新打开时继续同步。'); }
                }
                if (!live || version !== hostReadVersion) return;
                const context = await readCodexHostContext();
                if (!live || version !== hostReadVersion || token !== getCodexHostToken()) return;
                let next: ProjectDetail | undefined;
                if (useLegacyLink && context.projectId) {
                    next = await api<ProjectDetail>(`/projects/${context.projectId}`);
                }
                useLegacyLink = false;
                const list = await api<{projects: Project[]}>('/projects');
                if (!live || version !== hostReadVersion || token !== getCodexHostToken()) return;
                if (!next && !detailRef.current && list.projects.length) {
                    const last = new URL(window.location.href).searchParams.get('canvas') || localStorage.getItem('canvepp:last-project');
                    next = await api<ProjectDetail>(`/projects/${list.projects.some(p => p.id === last) ? last : list.projects[0].id}`);
                }
                if (!live || version !== hostReadVersion || token !== getCodexHostToken()) return;
                setProjects(list.projects);
                setHostContext(context);
                if (next && next.project.id !== detailRef.current?.project.id) {
                    ++loadRequest.current;
                    restore(next);
                }
            } catch (error) {
                if (live && version === hostReadVersion) notify((error as Error).message);
            } finally {
                if (live && version === hostReadVersion) setLoading(false);
            }
        };
        const onHostHashChange = () => {
            if (new URLSearchParams(window.location.hash.slice(1)).has('canveppHost')) { useLegacyLink=true; void refreshHostContext(); }
        };
        window.addEventListener('hashchange', onHostHashChange);
        void (async () => {
            try {
                consumeCodexHostToken();
                setMetadataPolicy(localStorage.getItem('canvepp:metadata-policy') === 'preserve' ? 'preserve' : 'strip');
                await refreshLocalSession();
                if (!live) return;
                sessionReady = true;
                await refreshHostContext();
            } catch (error) {
                if (live) { notify((error as Error).message); setLoading(false); }
            }
        })();
        return () => { live = false; hostReadVersion++; window.removeEventListener('hashchange', onHostHashChange); };
    }, []);
    useEffect(() => { setCompareIds([]); setCompareOpen(false); }, [activeId]);
    useEffect(() => {
        if (!detail?.project.id) return;
        localStorage.setItem('canvepp:last-project', detail.project.id);
        const url = new URL(window.location.href);
        url.searchParams.set('canvas',detail.project.id);
        window.history.replaceState(window.history.state,'',url.pathname+url.search+url.hash);
    }, [detail?.project.id]);
    const markHistory = useCallback((before: Graph) => { undoStack.current.push({ graph: clone(before), selection: clone(selectionRef.current) }); if (undoStack.current.length > 80)
        undoStack.current.shift(); redoStack.current = []; setUndoCount(undoStack.current.length); setRedoCount(0); }, []);
    const updateGraph = useCallback((next: Graph | ((g: Graph) => Graph), history = true) => {
        // A conflict review holds both originals until the user chooses its fields.
        if (pendingMerge.current) return;
        const before = graphRef.current;
        const raw = typeof next === 'function' ? next(before) : next;
        if (sameDraft({ name: '', graph: before }, { name: '', graph: raw })) return;
        if (history) markHistory(before);
        const reconciled = reconcileGraph(before, raw);
        graphRef.current = reconciled; setGraph(reconciled);
        dirty.current = true; changeVersion.current++; setSaveState('unsaved');
        persistDraft();
    }, [markHistory, persistDraft]);
    const finishDrag = useCallback(() => {
        const positions = positionsRef.current;
        const ended = dragActive.current || Object.keys(positions).length > 0;
        positionsRef.current = {}; dragActive.current = false;
        // Persistence always uses the latest event, independent of display commits.
        if (Object.keys(positions).length) {
            const current = graphRef.current;
            const nodes = current.nodes.map(node => {
                const point = positions[node.id];
                return point && (point.x !== node.position.x || point.y !== node.position.y) ? { ...node, position: point } : node;
            });
            if (nodes.some((node, index) => node !== current.nodes[index])) updateGraph({ ...current, nodes });
        }
        // A pending text edit may have had its debounce skipped while dragging,
        // even if the pointer returned to its original position.
        if (ended) setDragEndEpoch(epoch => epoch + 1);
        for (const resume of dragWaiters.current.splice(0)) resume();
    }, [updateGraph]);
    finishDragRef.current = finishDrag;
    // Keep the final overlay until canonical nodes commit. Clearing the external
    // store inside finishDrag would let its synchronous subscribers see old props.
    useLayoutEffect(() => {
        if (!dragActive.current) positionStore.clear();
    }, [graph, dragEndEpoch, positionStore]);
    const waitForDrag = useCallback(async () => {
        if (dragActive.current) await new Promise<void>(resolve => dragWaiters.current.push(resolve));
    }, []);
    const startDrag = useCallback(() => { if (!pendingMerge.current) dragActive.current = true; }, []);
    useEffect(() => {
        const flush = () => finishDragRef.current();
        window.addEventListener('blur', flush); window.addEventListener('pagehide', flush);
        return () => {
            window.removeEventListener('blur', flush); window.removeEventListener('pagehide', flush);
            // Fast refresh/unmount also preserves the final locally visible pose.
            flush();
        };
    }, []);
    const receiveRemote = useCallback((fresh: ProjectDetail, base = baseProject.current) => {
        const current = detailRef.current;
        if (!current || current.project.id !== fresh.project.id || !base) return;
        const local = { name: current.project.name.trim() || base.name, graph: sharedGraph(graphRef.current) };
        const result = mergeProjectDraft(base, local, fresh.project);
        if (!sameDraft(base, fresh.project)) {
            // Whole-graph undo snapshots from before a remote edit would erase
            // that edit. Start a new local history after synchronization.
            undoStack.current = []; redoStack.current = [];
            setUndoCount(0); setRedoCount(0);
        }
        const mergedGraph = { ...reconcileGraph(fresh.project.graph, result.merged.graph), viewport: graphRef.current.viewport };
        const review = result.conflicts.length ? { base, local, remote: fresh.project, conflicts: result.conflicts } : undefined;
        pendingMerge.current = review; setMergeReview(review);
        baseProject.current = fresh.project;
        const next = { ...fresh, project: { ...fresh.project, name: result.merged.name, graph: mergedGraph } };
        graphRef.current = mergedGraph; setGraph(mergedGraph);
        detailRef.current = next; setDetail(next);
        setProjects(ps => ps.map(p => p.id === fresh.project.id ? fresh.project : p));
        dirty.current = !!review || !sameDraft(fresh.project, next.project);
        changeVersion.current++;
        setSaveState(review ? 'conflict' : dirty.current ? 'unsaved' : 'saved');
        persistDraft();
    }, [persistDraft]);
    const save = useCallback(async (): Promise<Project> => {
        finishDragRef.current();
        if (savePromise.current) {
            await savePromise.current;
            if (dirty.current) return save();
            return detailRef.current!.project;
        }
        if (!detailRef.current || !baseProject.current) throw new Error('项目尚未加载');
        if (pendingMerge.current) throw new Error(MERGE_PENDING);
        if (!dirty.current) return detailRef.current.project;
        const projectId = detailRef.current.project.id;
        const task = (async () => {
            // Bound contention retries. A new server revision is always fetched;
            // never repeat an obsolete expectedRevision or a generation request.
            for (let attempt = 0; attempt < 4; attempt++) {
                const current = detailRef.current!, base = baseProject.current!;
                if (current.project.id !== projectId) throw new Error('保存期间项目已切换');
                if (pendingMerge.current) throw new Error(MERGE_PENDING);
                const sent = { ...base, name: current.project.name.trim() || base.name, graph: clone(sharedGraph(graphRef.current)) };
                if (sameDraft(base, sent)) {
                    // Reverting an edit also restores authoritative derived
                    // metadata (stale/configVersion), without writing a revision.
                    receiveRemote({ ...current, project: base }, sent); return base;
                }
                setSaveState('saving');
                try {
                    const saved = await api<Project>(`/projects/${projectId}/graph`, 'PATCH', { expectedRevision: base.graphRevision, graph: sent.graph, name: sent.name });
                    // Merge edits typed while this request was in flight onto its
                    // acknowledgement, using exactly the sent snapshot as base.
                    await waitForDrag();
                    receiveRemote({ ...detailRef.current!, project: saved }, sent);
                    if (pendingMerge.current) throw new Error(MERGE_PENDING);
                    if (!dirty.current) return saved;
                } catch (error) {
                    if (!(error instanceof RequestError) || error.status !== 409) throw error;
                    const fresh = await api<ProjectDetail>(`/projects/${projectId}`);
                    await waitForDrag();
                    receiveRemote(fresh);
                    if (pendingMerge.current) throw new Error(MERGE_PENDING);
                    if (!dirty.current) return fresh.project;
                }
            }
            // Keep pending edits durably in this tab; the regular debounce will
            // try again, instead of flooding the user with a stale-write error.
            throw new Error(SAVE_RETRY);
        })();
        savePromise.current = task;
        try { return await task; }
        catch (error) {
            setSaveState(pendingMerge.current ? 'conflict' : error instanceof Error && error.message === SAVE_RETRY ? 'unsaved' : 'error'); persistDraft(); throw error;
        } finally { savePromise.current = null; }
    }, [persistDraft, receiveRemote, waitForDrag]);
    saveCurrentRef.current = save;
    useEffect(() => {
        if (!dirty.current || saveState === 'conflict' || saveState === 'error') return;
        const t = setTimeout(() => { if (!dragActive.current) void save().catch(e => notify((e as Error).message)); }, 500);
        return () => clearTimeout(t);
    }, [graph, detail?.project.name, save, notify, saveState, dragEndEpoch]);
    useEffect(() => {
        const retry = () => { if (dirty.current && !pendingMerge.current) void save().catch(e => notify((e as Error).message)); };
        window.addEventListener('online', retry);
        return () => window.removeEventListener('online', retry);
    }, [save, notify]);
    useEffect(()=>{
        if(!trashOpen||!detail?.project.id)return;
        let cancelled=false;setTrashBusy(true);setTrashError('');
        api<{items:TrashedCard[]}>(`/projects/${detail.project.id}/trash`).then(value=>{if(!cancelled)setTrash(value.items);}).catch(error=>{if(!cancelled)setTrashError(error.message);}).finally(()=>{if(!cancelled)setTrashBusy(false);});
        return()=>{cancelled=true;};
    },[trashOpen,detail?.project.id,detail?.project.graphRevision,trashRefresh]);
    const restoreTrash = async (entryIds:string[]) => {
        if(trashBusy||projectBusy||pendingMerge.current||!detailRef.current)return;
        setTrashBusy(true);setProjectBusy(true);setTrashError('');
        try{
            const project=await save(),before={graph:clone(graphRef.current),selection:clone(selectionRef.current)};
            const result=await api<{project:Project;restoredIds:string[];skippedEdges:number}>(`/projects/${project.id}/trash/restore`,'POST',{entryIds,expectedRevision:project.graphRevision});
            receiveRemote({...detailRef.current!,project:result.project});
            undoStack.current.push(before);redoStack.current=[];setUndoCount(undoStack.current.length);setRedoCount(0);
            setActiveId(result.restoredIds[0]);setSelectedIds(result.restoredIds);setSelectedEdges([]);
            setTrashRefresh(value=>value+1);
            notify(`已恢复 ${result.restoredIds.length} 张图片${result.skippedEdges?'；缺少来源的连线仍保留在回收记录中':''}`);
        }catch(error){setTrashError((error as Error).message);}
        finally{setTrashBusy(false);setProjectBusy(false);}
    };
    const refreshRuns = useCallback(async () => {
        const currentId = detailRef.current?.project.id;
        if (!currentId || dragActive.current) return;
        try {
            const fresh = await api<ProjectDetail>(`/projects/${currentId}`);
            const current = detailRef.current;
            if (!current || current.project.id !== fresh.project.id) return;
            // An old poll may finish after a PATCH acknowledgement. Never move
            // the synchronization baseline backwards.
            if (!dragActive.current && !savePromise.current && !pendingMerge.current && fresh.project.graphRevision > (baseProject.current?.graphRevision ?? -1)) {
                receiveRemote(fresh);
            } else {
                const assets = JSON.stringify(current.assets) === JSON.stringify(fresh.assets) ? current.assets : fresh.assets;
                const runs = JSON.stringify(current.runs) === JSON.stringify(fresh.runs) ? current.runs : fresh.runs;
                const budget = JSON.stringify(current.budget) === JSON.stringify(fresh.budget) ? current.budget : fresh.budget;
                if (assets === current.assets && runs === current.runs && budget === current.budget) return;
                const next = { ...current, assets, runs, budget };
                detailRef.current = next; setDetail(next);
            }
        } catch { /* Keep durable task state and this tab's draft while offline. */ }
    }, [receiveRemote]);
    useEffect(() => { if (!detail) return; const t = setInterval(refreshRuns, 1800); return () => clearInterval(t); }, [detail?.project.id, refreshRuns]);
    const resolveMerge = (choice: 'local' | 'remote') => {
        const review = pendingMerge.current, current = detailRef.current;
        if (!review || !current) return;
        const choices = { ...review.resolutions, ...Object.fromEntries(review.conflicts.map(conflict => [conflict.path, choice])) };
        const result = resolveProjectDraftConflicts(review.base, review.local, review.remote, choices);
        if (result.conflicts.length) {
            // Retain all three originals and prior decisions when a structural
            // choice reveals a dependent conflict; never ask the same one again.
            const next = { ...review, resolutions: choices, conflicts: result.conflicts };
            pendingMerge.current = next; setMergeReview(next); persistDraft(); return;
        }
        pendingMerge.current = undefined; setMergeReview(undefined);
        const nextGraph = { ...reconcileGraph(review.remote.graph, result.merged.graph), viewport: graphRef.current.viewport };
        const next = { ...current, project: { ...review.remote, name: result.merged.name, graph: nextGraph } };
        graphRef.current = nextGraph; setGraph(nextGraph); detailRef.current = next; setDetail(next);
        baseProject.current = review.remote;
        dirty.current = !sameDraft(review.remote, next.project); changeVersion.current++;
        setSaveState(dirty.current ? 'unsaved' : 'saved'); persistDraft();
    };
    const active = graph.nodes.find(n => n.id === activeId);
    const assets = detail?.assets || EMPTY_ASSETS;
    const runs = detail?.runs || EMPTY_RUNS;
    const selectedRun=showRun?(runs.find(run=>run.id===showRun.id)||showRun):undefined;
    const activeAssetId = active ? getNodeOutputAsset(graph, active.id) : undefined;
    const activeAsset = assets.find(a => a.id === activeAssetId);
    const historyNodeId = active?.kind === 'output' ? graph.edges.find(e => e.targetNodeId === active.id && e.targetPort === 'image')?.sourceNodeId : active?.id;
    const referenceNodeId = graph.edges.find(e => e.targetNodeId === historyNodeId && e.targetPort === 'image')?.sourceNodeId;
    const referenceAssetId = referenceNodeId ? getNodeOutputAsset(graph, referenceNodeId) : undefined;
    const nodeAssets = useMemo(() => active ? assets.filter(a => a.nodeId === historyNodeId || a.id === activeAssetId || a.id === referenceAssetId || runs.some(r => r.nodeId === historyNodeId && r.assetVersionIds.includes(a.id))).sort((a, b) => b.createdAt.localeCompare(a.createdAt)) : [...assets].sort((a,b)=>b.createdAt.localeCompare(a.createdAt)), [active, assets, runs, activeAssetId, historyNodeId, referenceAssetId]);
    const changeNode = useCallback((nodeId: string, patch: Partial<CanvasNode>) => updateGraph(g => ({ ...g, nodes: g.nodes.map(n => n.id === nodeId ? { ...n, ...patch } : n) })), [updateGraph]);
    const requestUpload = useCallback((nodeId?: string) => { uploadTarget.current = nodeId; fileInput.current?.click(); }, []);
    const chooseAsset = useCallback((asset: AssetVersion) => { if (!active)
        return; const targetId = active.kind === 'output' ? graphRef.current.edges.find(e => e.targetNodeId === active.id && e.targetPort === 'image')?.sourceNodeId : active.id; if (!targetId) {
        notify('请先连接一个图片来源节点');
        return;
    } changeNode(targetId, { selectedAssetVersionId: asset.id, stale: false }); notify('已采用此版本，历史记录仍完整保留'); }, [active, changeNode, notify]);
    const prepareGroup = useCallback(async (sourceNodeIds: string[], prompt?: string, params?: ImageParams) => {
        if (previewBusy || loading || projectBusy || runBusy) return;
        const projectId = detailRef.current?.project.id;
        setPreviewBusy(true);
        try {
            let project = await save(); while (dirty.current) project = await save();
            if (project.id !== projectId) return;
            const next = await api<GenerationGroupPreview>('/generation-groups/preview', 'POST', { projectId, graphRevision: project.graphRevision, mode, sourceNodeIds, prompt, params, ...(mode==='codex' && {execution:'canvas'}) });
            if (detailRef.current?.project.id !== projectId) return;
            confirmationKey.current = id(); setAcceptStale(false); setGroupPreview(next);
        } catch (error) { notify((error as Error).message); }
        finally { setPreviewBusy(false); }
    }, [previewBusy, loading, projectBusy, runBusy, mode, save, notify]);
    const prepareRun = useCallback(async (nodeId: string) => { if (previewBusy || loading || projectBusy) return;
        const draftError = sizeDraftErrorsRef.current[nodeId];
        if (draftError) { notify(draftError); return; }
        const target = graphRef.current.nodes.find(node=>node.id===nodeId);
        // Every image creation reserves an independent result card, including
        // n=1. The legacy run path only retains versions on the source card.
        if(target && isImageCreator(target.kind) && mode!=='api'){await prepareGroup([nodeId]);return;}
        const originProject = detailRef.current?.project.id, token = getCodexHostToken();
        if (mode === 'codex' && (!hostContext.connected || hostContext.projectId !== originProject || !token)) { notify('请从这张画布所属的 Codex 会话打开后再生成。'); return; }
        
        const pending=detailRef.current?.runs.find(run=>!run.historyOnly && run.nodeId===nodeId&&(PENDING.includes(run.status)||(mode==='codex'&&run.mode==='codex'&&run.status==='unknown')));
        if(pending){setShowRun(pending);notify(pending.status==='unknown'?'请先检查当前 Codex 对话，确认已有任务的结果':'此节点已有待处理任务，请先查看或取消');return;}
        setPreviewBusy(true); try {
        let p = await save();
        while (dirty.current)
            p = await save();
        if (p.id !== originProject || token !== getCodexHostToken()) return;
        const next = await api<RunPreview>('/runs/preview', 'POST', { projectId: p.id, nodeId, graphRevision: p.graphRevision, mode }, token);
        if (detailRef.current?.project.id !== originProject || token !== getCodexHostToken()) return;
        previewHostToken.current = token;
        setPreview(next);
        setAcceptStale(false);
        confirmationKey.current = id();
    }
    catch (e) {
        notify((e as Error).message);
    }
    finally {
        setPreviewBusy(false);
    } }, [previewBusy, save, mode, notify, loading, projectBusy, hostContext, prepareGroup]);
    const copyCodexHandoff=useCallback(async(run:Run)=>{
        const checkOnly=run.status!=='awaiting_host'||!!hostDispatchStatus(run,hostDispatches[run.id]);
        const message=checkOnly?`请检查 Canvepp 的 Codex 原生图像任务：${run.id}。先核实已有任务状态和当前对话中已经执行的图像工具记录。若已有结果，请回填同一 runId；不要重新调用生图工具、创建任务或发送重复请求。`:`请处理 Canvepp 已确认的 Codex 原生图像任务：${run.id}。先读取冻结快照并原子领取任务，仅在领取返回 canGenerate:true 时调用一次 Codex 原生图像工具，然后将结果回填同一 runId。不要创建重复任务或重复生成。`;
        try{await navigator.clipboard.writeText(message);notify('交接指令已复制，请粘贴到当前 Codex 对话');}
        catch{downloadText(`canvepp-${run.id}-handoff.txt`,message);notify('无法写入剪贴板，已下载交接指令');}
    },[notify,hostDispatches]);
    const dispatchCodexRun=useCallback(async(run:Run)=>{
        const token=getCodexHostToken();
        if(!hostContext.connected||hostContext.projectId!==run.projectId||!token){notify('请从 Codex 打开画布以连接当前对话；任务仍在等待处理');setShowRun(run);return;}
        if(hostDispatchStatus(run,hostDispatches[run.id])){setShowRun(run);notify(hostDispatchStatus(run,hostDispatches[run.id])==='unknown'?'交接结果尚不确定，请先检查当前 Codex 对话':'此任务已经交接，请等待 Codex 处理');return;}
        if(hostDispatchInFlight.current.has(run.id))return;
        hostDispatchInFlight.current.add(run.id);setHostDispatchBusy(run.id);
        try{
            const receipt=await api<CodexDispatchReceipt>(`/runs/${run.id}/host/dispatch`,'POST',{hostContext:token},token);
            setHostDispatches(current=>({...current,[run.id]:receipt.status}));
            if(receipt.status==='unknown'){setShowRun(run);notify('交接结果尚不确定，请先检查当前 Codex 对话，勿重复交接');}
            else notify(receipt.status==='already_queued'?'此任务已排入 Codex 对话，等待处理':'请求已排入当前 Codex 对话，等待开始处理');
            await refreshRuns();
        }catch(error){
            // A lost response can follow a successful queue operation. Do not offer a blind resend.
            if(!(error instanceof RequestError)||error.status>=500)setHostDispatches(current=>({...current,[run.id]:'unknown'}));
            if(error instanceof RequestError&&error.status===403)setHostContext({connected:false});
            setShowRun(run);notify(`任务交接未确认：${(error as Error).message}`);
        }
        finally{hostDispatchInFlight.current.delete(run.id);setHostDispatchBusy(undefined);}
    },[hostContext,hostDispatches,notify,refreshRuns]);
    const confirmGroup = async () => {
        if (!groupPreview || runBusy) return;
        const currentPreview = groupPreview;
        if (currentPreview.projectId !== detailRef.current?.project.id) { setGroupPreview(undefined); notify('画布已切换，请重新预览'); return; }
        setRunBusy(true);
        try {
            const result = await api<GenerationGroupResult>('/generation-groups', 'POST', { previewId: currentPreview.id, snapshotHash: currentPreview.snapshotHash, confirmationKey: confirmationKey.current, acceptStale });
            if (detailRef.current?.project.id !== currentPreview.projectId) return;
            setGroupPreview(undefined); await refreshRuns();

            setTimeout(()=>flow.fitView({nodes:[...currentPreview.sourceNodeIds,...result.runs.map(run=>run.nodeId)].map(id=>({id})),padding:.25,maxZoom:1,duration:250}),100);
            notify(`已创建 ${result.runs.length} 张独立结果图片，原图保留`);
            if (currentPreview.mode === 'manual') openRunHistory();
        } catch (error) { notify((error as Error).message); }
        finally { setRunBusy(false); }
    };
    const confirmRun = async () => { if (!preview || runBusy)
        return;
        const expectedProject = preview.projectId, token = previewHostToken.current;
        if (expectedProject !== detailRef.current?.project.id || token !== getCodexHostToken()) { setPreview(undefined); notify('会话或画布已切换，请重新预览。'); return; }
        setRunBusy(true); try {
        const run = await api<Run>('/runs', 'POST', { previewId: preview.id, snapshotHash: preview.snapshotHash, confirmationId: confirmationKey.current, idempotencyKey: confirmationKey.current, acceptStale }, token);
        if (detailRef.current?.project.id !== expectedProject || token !== getCodexHostToken()) return;
        setDetail(d => { if (!d)
            return d; const next = { ...d, runs: [run, ...d.runs.filter(r => r.id !== run.id)] }; detailRef.current = next; return next; });
        setPreview(undefined);
        openRunHistory();
        if (run.status === 'awaiting_import') {
            setAttachRun(run);
            setAttachAssets([]);
            setSourceNote('');
            setAttachConfirm(false);
        }
        if(run.mode==='codex'){if(hostContext.connected&&getCodexHostToken())await dispatchCodexRun(run);else{setShowRun(run);notify('任务已创建并等待处理，请从 Codex 打开画布以连接当前对话');}}
        else notify(run.mode === 'manual' ? '任务已保存，可在外部生成后回填' : '任务已创建，只运行所选节点');
        await refreshRuns();
    }
    catch (e) {
        notify((e as Error).message);
    }
    finally {
        setRunBusy(false);
    } };
    const projectId = detail?.project.id;
    const providerModel = mode==='api'?detail?.budget.settings.model:undefined;
    // Selection, viewport and polling metadata never trigger per-card graph validation.
    // The batch resolver validates the graph once and reports each runnable input.
    const runInputs = useMemo<ReturnType<typeof resolveGraphInputs>>(() => {
        if(!detail) return {};
        return resolveGraphInputs({...detail.project,graph},assets,mode==='codex'?'manual':mode,providerModel);
    },[projectId,graph,assets,mode,providerModel]);
    const budget = detail?.budget;
    const runReasons = useMemo<Record<string,string|undefined>>(() => {
        const reasons:Record<string,string|undefined> = {};
        for(const node of graph.nodes){
            if(!isImageCreator(node.kind))continue;
            if(sizeDraftErrors[node.id]){reasons[node.id]=sizeDraftErrors[node.id];continue;}
            const effectiveParams=getEditorInputs(graph,node).params;
            if(mode==='api'&&!(API_IMAGE_SIZES as readonly string[]).includes(effectiveParams.size)){reasons[node.id]='当前 API 适配器不支持此尺寸，请选择 1:1、3:2 或 2:3';continue;}
            const input=runInputs[node.id];
            if(!input?.snapshot){reasons[node.id]=input?.error||'项目尚未加载';continue;}
            if(mode==='codex'&&runs.some(run=>!run.historyOnly && run.nodeId===node.id&&run.mode==='codex'&&run.status==='unknown')){reasons[node.id]='已有任务待确认，请在历史中核查结果';continue;}
            if(runs.some(run=>!run.historyOnly && generationGroup(run)?.sourceNodeIds.includes(node.id)&&[...PENDING,'unknown'].includes(run.status))){reasons[node.id]='已有一组图片正在生成或等待确认';continue;}
            if(mode!=='api')continue;
            if(!budget){reasons[node.id]='项目尚未加载';continue;}
            const s=budget.settings;
            const cost=input.snapshot.params.n*s.pricePerImage;
            reasons[node.id]=!s.apiEnabled?'请先在设置中启用 API 并完成预算配置':
                !budget.keyConfigured?'尚未配置服务端 API 密钥，请使用 Codex 原生、模拟或手动模式':
                !s.organization||!s.project?'请先核实 API 组织与项目':
                !s.creditsConfirmed||!s.paidBalanceAcknowledged||!s.autoRechargeAcknowledged?'请在设置中确认额度、余额与自动充值状态':
                !s.creditsExpireAt||Date.parse(s.creditsExpireAt)<=Date.now()?'额度已到期或未知，请重新核实':
                !s.priceValidUntil||Date.parse(s.priceValidUntil)<=Date.now()||s.pricePerImage<=0?'价格上界未知或已到期，无法提交':
                cost>s.perRunLimit?'本次费用上界超过每次预算':
                cost+budget.reserved+budget.spent>s.monthlyLimit?'本月预算不足':
                cost+budget.reserved+budget.spent>s.availableCredits?'已核实额度不足':
                '模型与费用上界待校准，真实 API 生成暂不可用';
        }
        return reasons;
    },[graph,runInputs,mode,budget,runs,sizeDraftErrors,hostContext,projectId]);
    const runReason = useCallback((node:CanvasNode)=>runReasons[node.id],[runReasons]);
    const highResolutionCards = zoom > 150;
    const cardData = useMemo<Record<string,CardData>>(() => {
        const byId=new Map(assets.map(asset=>[asset.id,asset]));
        const nodesById=new Map(graph.nodes.map(node=>[node.id,node]));
        const latestRuns=new Map<string,Run>();
        for(const run of runs)if(!latestRuns.has(run.nodeId))latestRuns.set(run.nodeId,run);
        const paramsSources=new Map(graph.edges.filter(edge=>edge.targetPort==='params').map(edge=>[edge.targetNodeId,edge.sourceNodeId]));
        const connectedNodes=new Set(graph.edges.flatMap(edge=>[edge.sourceNodeId,edge.targetNodeId]));
        const data:Record<string,CardData>={};
        for(const item of graph.nodes){
            data[item.id]={
                showPorts:connectedNodes.has(item.id),
                inputPortIds:graph.edges.filter(edge=>edge.targetNodeId===item.id&&!['reference','origin'].includes(edge.targetPort)).map(edge=>edge.targetPort),
                outputPortIds:graph.edges.filter(edge=>edge.sourceNodeId===item.id&&!['reference','origin'].includes(edge.targetPort)).map(edge=>edge.sourcePort),
                highResolution:highResolutionCards,
                item,asset:byId.get(getNodeOutputAsset(graph,item.id)||''),run:latestRuns.get(item.id),
                versionCount:assets.filter(asset=>asset.nodeId===item.id||asset.id===item.selectedAssetVersionId).length,
                resolvedCount:runInputs[item.id]?.snapshot?.params.n||nodesById.get(paramsSources.get(item.id)||'')?.config.params?.n||item.config.params?.n||1,
                onRun:prepareRun,onView:setViewer,onUpload:requestUpload,
                onSelect:(nodeId)=>{setActiveId(nodeId);setSelectedIds([nodeId]);openInspector();},
                onHistory:(nodeId)=>{setActiveId(nodeId);setSelectedIds([nodeId]);setVersionsOpen(true);setHistoryView('images');openHistory();},
                runReason:runReasons[item.id],
                onExport:(assetId)=>{const asset=byId.get(assetId);if(asset)downloadAsset(asset);}
            };
        }
        return data;
    },[graph,assets,runs,runInputs,runReasons,prepareRun,requestUpload,highResolutionCards,openHistory,openInspector]);
    const cards = useMemo<CardNode[]>(()=>{
        const selected=new Set(selectedIds);
        return graph.nodes.map(item=>({id:item.id,type:'canvas',position:item.position,measured:nodeMeasurements[item.id],selected:selected.has(item.id),data:cardData[item.id]}));
    },[graph,nodeMeasurements,selectedIds,cardData]);
    const flowEdges = useMemo(() => graph.edges.map(edge => ({ id: edge.id, source: edge.sourceNodeId, target: edge.targetNodeId, sourceHandle: ['reference','origin'].includes(edge.targetPort)?'generation-source':edge.sourcePort, targetHandle: ['reference','origin'].includes(edge.targetPort)?'generation-target':edge.targetPort, type: 'default', selected: selectedEdges.includes(edge.id), markerEnd: { type: MarkerType.ArrowClosed, width: 15, height: 15, color: '#a6a6a6' }, style: { stroke: edge.targetPort === 'prompt' ? '#9f9f9f' : edge.targetPort === 'params' ? '#9c9c9c' : '#939393', strokeWidth: 1.8 }, interactionWidth: 20 })), [graph.edges, selectedEdges]);
    const onNodesChange = useCallback((changes: NodeChange<CardNode>[]) => {
        const dimensionChanges = changes.filter(change => change.type === 'dimensions');
        if (dimensionChanges.length) setNodeMeasurements(previous => {
            let next = previous;
            for (const change of dimensionChanges) {
                if (change.type !== 'dimensions' || !change.dimensions) continue;
                const {width,height} = change.dimensions;
                if (!Number.isFinite(width) || !Number.isFinite(height) || width <= 0 || height <= 0) continue;
                const old = previous[change.id];
                if (old?.width === width && old.height === height) continue;
                if (next === previous) next = {...previous};
                next[change.id] = {width,height};
            }
            return next;
        });
        const selectionChanges = changes.filter(c => c.type === 'select');
        if (selectionChanges.length) setSelectedIds(previous => {
            const next = new Set(previous);
            for (const change of selectionChanges) {
                if (change.type !== 'select') continue;
                if (change.selected) next.add(change.id); else next.delete(change.id);
            }
            const ids = [...next];
            return equalIds(previous, ids) ? previous : ids;
        });
        const positionChanges = changes.filter(c => c.type === 'position');
        if (!positionChanges.length || pendingMerge.current) return;
        if (positionChanges.some(change => change.type === 'position' && change.dragging === true)) startDrag();
        for (const change of positionChanges) {
            if (change.type === 'position' && change.position) positionsRef.current[change.id] = change.position;
        }
        // Keyboard moves have no drag start; aborted pointer drags may only emit
        // dragging:false, with no onNodeDragStop. Both use the same idempotent end.
        if (!dragActive.current || positionChanges.some(change => change.type === 'position' && change.dragging === false)) {
            finishDrag(); return;
        }
        // Only the canvas subscribes; sidebar/workspace do not render per move.
        // React Flow already schedules drag events; adding another RAF adds lag.
        positionStore.publish(positionsRef.current);
    }, [finishDrag, startDrag, positionStore]);
    const onEdgesChange = useCallback((changes: EdgeChange[]) => {
        const selections = changes.filter(c => c.type === 'select');
        if (selections.length) setSelectedEdges(previous => {
            const next = new Set(previous);
            for (const change of selections) {
                if (change.type !== 'select') continue;
                if (change.selected) next.add(change.id); else next.delete(change.id);
            }
            const ids = [...next];
            return equalIds(previous, ids) ? previous : ids;
        });
        const removed = changes.filter(c => c.type === 'remove').map(c => c.id);
        if (removed.length) updateGraph(g => ({ ...g, edges: g.edges.filter(e => !removed.includes(e.id)) }));
    }, [updateGraph]);
    const onSelectionChange = useCallback(({ nodes, edges }: { nodes:CardNode[]; edges:{id:string}[] }) => {
        const nodeIds = nodes.map(node => node.id);
        const edgeIds = edges.map(edge => edge.id);
        // Observe only: selection deltas above are the single writer. React
        // Flow reports node/edge snapshots at different points during marquee
        // selection; feeding those snapshots back creates an update loop.
        if (nodeIds.length === 1) setActiveId(previous => previous === nodeIds[0] ? previous : nodeIds[0]);
        else if (edgeIds.length && !nodeIds.length) setActiveId(previous => previous === undefined ? previous : undefined);
    }, []);
    const connectionEdge = useCallback((connection: Connection): CanvasEdge => ({ id: id(), sourceNodeId: connection.source!, sourcePort: connection.sourceHandle || 'out', targetNodeId: connection.target!, targetPort: connection.targetHandle || 'image' }), []);
    const onConnect = useCallback((connection: Connection) => { const edge = connectionEdge(connection); const result = validateConnection(graphRef.current, edge); if (!result.valid) {
        notify(result.errors[0]);
        return;
    } updateGraph(g => ({ ...g, edges: [...g.edges, edge] })); }, [connectionEdge, updateGraph, notify]);
    const onConnectEnd = useCallback<NonNullable<ReactFlowProps<CardNode>['onConnectEnd']>>((_, state) => {
        if (state.isValid === false && state.fromNode && state.toNode) {
            const result = validateConnection(graphRef.current, { id: id(), sourceNodeId: state.fromNode.id, sourcePort: state.fromHandle?.id || 'out', targetNodeId: state.toNode.id, targetPort: state.toHandle?.id || 'image' });
            if (!result.valid) notify(result.errors[0]);
        }
    }, [notify]);
    const isValidConnection = useCallback<NonNullable<ReactFlowProps<CardNode>['isValidConnection']>>(connection => validateConnection(graphRef.current, connectionEdge(connection as Connection)).valid, [connectionEdge]);
    const onNodeClick = useCallback<NonNullable<ReactFlowProps<CardNode>['onNodeClick']>>((_, node) => { setActiveId(node.id); if(!historyOpenRef.current) openInspector(); }, [openInspector]);
    const onPaneClick = useCallback(() => { setActiveId(undefined); setSelectedIds([]); setSelectedEdges([]); workspaceRef.current?.focus(); }, []);
    const onEdgeClick = useCallback(() => { setActiveId(undefined); setSelectedIds([]); }, []);
    const onMoveEnd = useCallback<NonNullable<ReactFlowProps<CardNode>['onMoveEnd']>>((_, viewport) => {
        setZoom(Math.round(viewport.zoom * 100));
        if (detailRef.current) { graphRef.current = { ...graphRef.current, viewport }; writeViewport(detailRef.current.project.id, viewport); }
    }, []);
    const revealNewNode = useCallback((nodeId: string) => {
        // Wait for React Flow to measure the added card before fitting its bounds.
        setTimeout(() => flow.fitView({ nodes: [{ id: nodeId }], padding: .4, maxZoom: Math.min(flow.getZoom(), 1), duration: 250 }), 80);
    }, [flow]);
    const addNode = useCallback((kind: NodeKind, preferredPosition?:{x:number;y:number}) => {
        if (!detailRef.current) return;
        const center = flow.screenToFlowPosition({ x: window.innerWidth * .43, y: window.innerHeight * .43 });
        const position = findOpenPosition(graphRef.current.nodes, preferredPosition || { x: center.x - 124, y: center.y - 70 }, nodeMeasurements);
        const node = createNode(kind, detailRef.current.project.id, position);
        if (kind === 'generate') node.name = '新建图片';
        updateGraph(g => ({ ...g, nodes: [...g.nodes, node] }));
        setActiveId(node.id);setSelectedIds([node.id]);openInspector();
        if(!preferredPosition)revealNewNode(node.id);
        if (kind === 'image') requestUpload(node.id);
    }, [flow, updateGraph, requestUpload, nodeMeasurements, revealNewNode]);
    const restoreSelection = useCallback((selection: CanvasSelection, next: Graph) => {
        const nodeIds = selection.nodeIds.filter(id => next.nodes.some(node => node.id === id));
        const edgeIds = selection.edgeIds.filter(id => next.edges.some(edge => edge.id === id));
        const activeId = next.nodes.some(node => node.id === selection.activeId) ? selection.activeId : nodeIds[0];
        selectionRef.current = { nodeIds, edgeIds, activeId };
        setSelectedIds(nodeIds); setSelectedEdges(edgeIds); setActiveId(activeId);
    }, []);
    const travelHistory = useCallback((direction: 'undo' | 'redo') => {
        if (pendingMerge.current || !detailRef.current) return;
        finishDragRef.current();
        const from = direction === 'undo' ? undoStack : redoStack;
        const to = direction === 'undo' ? redoStack : undoStack;
        const entry = from.current.at(-1);
        if (!entry) return;
        const next = restoreGraphSnapshot(graphRef.current, entry.graph, detailRef.current.project.id);
        const current = { graph: clone(graphRef.current), selection: clone(selectionRef.current) };
        updateGraph(next, false);
        from.current.pop(); to.current.push(current);
        restoreSelection(entry.selection, next);
        setUndoCount(undoStack.current.length); setRedoCount(redoStack.current.length);
    }, [updateGraph, restoreSelection]);
    const undo = useCallback(() => travelHistory('undo'), [travelHistory]);
    const redo = useCallback(() => travelHistory('redo'), [travelHistory]);
    const copyNodes = useCallback((nodeIds?:string[]) => {
        if (pendingMerge.current || !detailRef.current) return;
        finishDragRef.current();
        const copied=copySelection(graphRef.current,detailRef.current.project.id,nodeIds||selectionRef.current.nodeIds);
        if(!copied)return;
        clipboard.current=copied;pasteIndex.current=0;
        const assetId=copied.nodes.length===1?getNodeOutputAsset(graphRef.current,copied.nodes[0].id):undefined;
        clipboardPending.current=copied;clipboardOwned.current=true;clipboardLocalOnly.current=false;
        void writeCanvasClipboard(copied,assetId).then(()=>notify(`已复制 ${copied.nodes.length} 张图片`)).catch(()=>{clipboardLocalOnly.current=true;notify('已在画布内复制，可用右键或 ⌘V 粘贴');}).finally(()=>{if(clipboardPending.current===copied)clipboardPending.current=undefined;});
    },[notify]);
    const pasteNodes = useCallback((copied=clipboard.current,point?:{x:number;y:number}) => {
        if(pendingMerge.current||!detailRef.current||!copied)return;
        finishDragRef.current();
        try{
            const offset=point?{x:point.x-Math.min(...copied.nodes.map(node=>node.position.x)),y:point.y-Math.min(...copied.nodes.map(node=>node.position.y))}:undefined;
            const pasted=pasteSelection(graphRef.current,copied,{projectId:detailRef.current.project.id,pasteIndex:pasteIndex.current+1,offset});
            if(!pasted.nodeIds.length)return;
            updateGraph(pasted.graph);clipboard.current=copied;pasteIndex.current++;
            restoreSelection({nodeIds:[...pasted.nodeIds,...pasted.dependencyNodeIds],edgeIds:pasted.edgeIds,activeId:pasted.nodeIds[0]},pasted.graph);
            notify(`已粘贴 ${pasted.nodeIds.length} 张图片`);
        }catch(error){notify((error as Error).message);}
    },[updateGraph,restoreSelection,notify]);
    const duplicateActive = () => {
        if (!active || !detailRef.current || pendingMerge.current) return;
        finishDragRef.current();
        clipboardOwned.current=false;clipboard.current = copySelection(graphRef.current, detailRef.current.project.id, [active.id]); pasteIndex.current = 0;
        pasteNodes();
    };
    const deleteNodes = async () => {
        if(!deleteTarget||pendingMerge.current||deleting.current)return;
        deleting.current=true;setDeleteBusy(true);
        try{
            // Persist even newly created cards before archiving them on the server.
            await save();
            const next=deleteSelection(graphRef.current,deleteTarget.nodeIds,deleteTarget.edgeIds).graph;
            updateGraph(next);restoreSelection({nodeIds:[],edgeIds:[]},next);
            await save();setDeleteTarget(undefined);setTrashRefresh(value=>value+1);notify('已移到回收站');
            workspaceRef.current?.focus();
        }catch(error){notify((error as Error).message);}
        finally{deleting.current=false;setDeleteBusy(false);}
    };
    const keyboard = useCallback((e: React.KeyboardEvent) => { const target = e.target as HTMLElement; if (loading || projectBusy || pendingMerge.current || target.closest('input,textarea,select,button,summary,[contenteditable="true"]'))
        return; const cmd = e.metaKey || e.ctrlKey; if (cmd && e.key.toLowerCase() === 's') {
        e.preventDefault();
        save().then(() => notify('已保存到本地')).catch(e => notify(e.message));
    } if (cmd && e.key.toLowerCase() === 'z') {
        e.preventDefault();
        e.shiftKey ? redo() : undo();
    } if (cmd && e.key.toLowerCase() === 'c') {
        e.preventDefault();
        copyNodes();

    } if (e.key === 'Delete' || e.key === 'Backspace') {
        e.preventDefault();
        const ids = selectedIds;
        if (ids.length)
            setDeleteTarget({ nodeIds: ids, edgeIds: selectedEdges });
        else if (selectedEdges.length)
            { updateGraph(g => deleteSelection(g, [], selectedEdges).graph); setSelectedEdges([]); }
    } if (e.key === 'Escape') {
        setActiveId(undefined);
        setSelectedIds([]); setSelectedEdges([]);
    } if (e.key === 'Enter' && active && isImageCreator(active.kind)) {
        e.preventDefault();
        prepareRun(active.id);
    } }, [save, notify, redo, undo, copyNodes, selectedIds, activeId, selectedEdges, updateGraph, active, prepareRun, loading, projectBusy]);
    const importFiles = async (files: FileList | File[] | null, preferredPosition?:{x:number;y:number},insert=false) => {
        const current=detailRef.current;
        if(!files?.length||!current||importing.current||pendingMerge.current)return;
        const target=insert?undefined:uploadTarget.current,manualRun=insert?undefined:attachRun;
        const projectId=current.project.id,token=getCodexHostToken();
        const selected=Array.from(files).slice(0,50);
        importing.current=true;setUploading(true);const added:CanvasNode[]=[],failures:string[]=[];let imported=0;
        try{
            await save();
            for(const file of selected){
                if(detailRef.current?.project.id!==projectId)throw Error('画布已切换，已导入图片保留在原画布历史中');
                try{
                    const fd=new FormData();fd.set('file',file);fd.set('projectId',projectId);fd.set('metadataPolicy',metadataPolicy);if(target)fd.set('nodeId',target);
                    const asset=await api<AssetVersion>('/assets/import','POST',fd,token);
                    const fresh=detailRef.current;if(!fresh||fresh.project.id!==projectId)throw Error('画布已切换');
                    const next={...fresh,assets:[asset,...fresh.assets]};detailRef.current=next;setDetail(next);imported++;
                    if(manualRun)setAttachAssets(old=>[...old,asset.id]);
                    else if(target)changeNode(target,{selectedAssetVersionId:asset.id});
                    else{
                        const preferred=preferredPosition||flow.screenToFlowPosition({x:window.innerWidth*.4,y:window.innerHeight*.4});
                        const node=createNode('image',projectId,findOpenPosition([...graphRef.current.nodes,...added],preferred,nodeMeasurements));
                        node.selectedAssetVersionId=asset.id;node.name=file.name.replace(/\.[^.]+$/,'');added.push(node);
                    }
                }catch(error){failures.push(`${file.name}：${(error as Error).message}`);}
            }
            if(added.length){updateGraph(g=>({...g,nodes:[...g.nodes,...added]}));restoreSelection({nodeIds:added.map(node=>node.id),edgeIds:[],activeId:added[0].id},graphRef.current);openInspector();if(!preferredPosition)revealNewNode(added[0].id);}
            else if(target&&imported){setActiveId(target);setSelectedIds([target]);openInspector();}
            await save();
            notify(failures.length?`已导入 ${imported} 张；${failures[0]}`:`已插入 ${imported} 张图片${files.length>50?'，每次最多导入 50 张':''}`);
        }catch(error){notify((error as Error).message);}
        finally{importing.current=false;setUploading(false);if(fileInput.current)fileInput.current.value='';uploadTarget.current=undefined;}
    };
    const importTransfer = async (data:DataTransfer,point?:{x:number;y:number}) => {
        const files=imageFiles(data),urls=transferImageUrls(data);
        if(files.length){await importFiles(files,point,true);return;}
        if(!urls.length){notify('请拖入 PNG、JPEG 或 WebP 图片');return;}
        const loaded=await Promise.allSettled(urls.map(fileFromImageUrl));
        const usable=loaded.flatMap(result=>result.status==='fulfilled'?[result.value]:[]);
        if(usable.length)await importFiles(usable,point,true);
        if(loaded.some(result=>result.status==='rejected'))notify('部分网页图片不允许直接读取，请先保存图片后拖入');
    };
    const pasteEvent = (event:React.ClipboardEvent) => {
        if((event.target as HTMLElement).closest('input,textarea,select,[contenteditable="true"]')||loading||projectBusy||pendingMerge.current)return;
        if(clipboardOwned.current&&(clipboardPending.current||clipboardLocalOnly.current)){event.preventDefault();pasteNodes(clipboardPending.current||clipboard.current);return;}
        const data=event.clipboardData,copied=decodeCanvasClipboard(data.getData('text/plain'));
        if(copied){event.preventDefault();pasteNodes(copied);return;}
        if(imageFiles(data).length||transferImageUrls(data).length){event.preventDefault();void importTransfer(data);return;}
        if(!data.getData('text/plain')&&clipboard.current){event.preventDefault();pasteNodes();}
    };
    const pasteFromMenu = async (point:{x:number;y:number}) => {
        if(clipboardOwned.current&&clipboard.current){pasteNodes(clipboard.current,point);return;}
        try{const data=await readCanvasClipboard();if(data.canvas)pasteNodes(data.canvas,point);else if(data.files.length)await importFiles(data.files,point,true);else notify('剪贴板里没有图片');}
        catch{workspaceRef.current?.focus();notify('请按 ⌘V 粘贴图片');}
    };
    const showContextMenu = (event:React.MouseEvent) => {
        if(loading||projectBusy||pendingMerge.current||!detailRef.current)return;
        if((event.target as HTMLElement).closest('input,textarea,select,[contenteditable="true"],.canvas-bottom-toolbar'))return;
        event.preventDefault();finishDragRef.current();
        const nodeId=(event.target as HTMLElement).closest<HTMLElement>('.react-flow__node')?.dataset.id;
        const nodeIds=nodeId?(selectionRef.current.nodeIds.includes(nodeId)?selectionRef.current.nodeIds:[nodeId]):[];
        if(nodeIds.length)restoreSelection({nodeIds,edgeIds:[],activeId:nodeId},graphRef.current);
        setContextMenu({x:event.clientX,y:event.clientY,point:flow.screenToFlowPosition({x:event.clientX,y:event.clientY}),nodeIds});
    };
    const contextActions:CanvasMenuAction[]=contextMenu?[
        ...(contextMenu.nodeIds.length?[{label:'复制',shortcut:'⌘C',icon:<Copy size={15}/>,onClick:()=>copyNodes(contextMenu.nodeIds)}]:[{label:'新建图片',icon:<Plus size={15}/>,onClick:()=>addNode('generate',contextMenu.point)}]),
        {label:'粘贴',shortcut:'⌘V',icon:<FilePlus2 size={15}/>,onClick:()=>{void pasteFromMenu(contextMenu.point);}},
        ...(contextMenu.nodeIds.length?[{label:'删除',shortcut:'⌫',icon:<Trash2 size={15}/>,onClick:()=>setDeleteTarget({nodeIds:contextMenu.nodeIds,edgeIds:[]}),danger:true}]:[]),
    ]:[];
    const branchFrom = useCallback((asset: AssetVersion) => {
        if (!detailRef.current || asset.projectId !== detailRef.current.project.id) return;
        const source = graphRef.current.nodes.find(node => node.id === activeId);
        const pos = source?.position || { x: 100, y: 100 };
        const position = findOpenPosition(graphRef.current.nodes, { x: pos.x + 340, y: pos.y }, nodeMeasurements);
        const branch = createNode('image', detailRef.current.project.id, position);
        const sourceRun = detailRef.current.runs.find(run => run.id === asset.runId);
        branch.name = '继续编辑 · ' + (asset.name || '图片');
        branch.selectedAssetVersionId = asset.id;
        branch.config = { ...branch.config, text: '', params: { ...(sourceRun?.inputSnapshot.params || (source ? getEditorInputs(graphRef.current, source).params : DEFAULT_PARAMS)) } };
        updateGraph(g => ({ ...g, nodes: [...g.nodes, branch] }));
        setActiveId(branch.id);setSelectedIds([branch.id]);openInspector();
        revealNewNode(branch.id);
        notify('已建立独立图片分支，填写修改要求后再预览');
    }, [activeId, updateGraph, notify, nodeMeasurements, revealNewNode]);
    const updateAsset = async (asset: AssetVersion, patch: Partial<AssetVersion>) => { try {
        const updated = await api<AssetVersion>(`/assets/${asset.id}`, 'PATCH', patch);
        setDetail(d => { if (!d)
            return d; const next = { ...d, assets: d.assets.map(a => a.id === updated.id ? updated : a) }; detailRef.current = next; return next; });
    }
    catch (e) {
        notify((e as Error).message);
    } };
    const exportProject = async () => { try {
        const p = await save();
        const a = document.createElement('a');
        a.href = `/api/projects/${p.id}/export`;
        a.download = `${p.name}.canvepp.zip`;
        a.click();
    }
    catch (e) {
        notify((e as Error).message);
    } };
    const switchCanvas = async (projectId: string) => {
        if (loading || projectBusy || uploading || runBusy) return;
        setProjectMenu(false);
        try {
            if (detailRef.current) await save();
            setCanvasPickerOpen(false);
            await loadProject(projectId);
        } catch (error) { notify((error as Error).message); }
    };
    const createCanvas = async () => {
        if (loading || projectBusy || uploading || runBusy || mutationBusy.current) return;
        setProjectBusy(true);
        try {
            if (detailRef.current) await save();
            const next = await api<ProjectDetail>('/projects','POST',{name:'未命名画布',template:'image'});
            ++loadRequest.current;
            restore(next);
            setProjects(list => [next.project,...list]);
            setCanvasPickerOpen(false);
        } catch (error) { notify((error as Error).message); }
        finally { setProjectBusy(false); }
    };
    const toggleCanvasPicker = () => {
        setCanvasPickerOpen(open=>!open); setProjectMenu(false);
        if (!canvasPickerOpen) void api<{projects:Project[]}>('/projects').then(list=>setProjects(list.projects)).catch(error=>notify(error.message));
    };
    const importProject = async (file?: File) => {
        if (!file || projectBusy || !detailRef.current) return;
        setProjectBusy(true);
        try {
            const current = await save();
            const before = { graph: clone(graphRef.current), selection: clone(selectionRef.current) };
            const past = [...undoStack.current];
            const form = new FormData();
            form.set('file', file);
            form.set('expectedRevision', String(current.graphRevision));
            const next = await api<ProjectDetail>(`/projects/${current.id}/restore`, 'POST', form);
            if (detailRef.current?.project.id !== current.id) return;
            restore(next);
            undoStack.current = [...past, before].slice(-80);
            setUndoCount(undoStack.current.length);
            setProjects(list => list.map(p => p.id === next.project.id ? next.project : p));
            setRestoreOpen(false);
            notify('备份内容已加入当前画布，原有图片和历史完整保留。');
        } catch (error) { notify((error as Error).message); }
        finally { setProjectBusy(false); if (packageInput.current) packageInput.current.value = ''; }
    };
    const renameProject = (name: string) => {
        const current = detailRef.current;
        if (!current || pendingMerge.current || name === current.project.name) return;
        const next = { ...current, project: { ...current.project, name } };
        detailRef.current = next; setDetail(next); dirty.current = true; changeVersion.current++; setSaveState('unsaved'); persistDraft();
    };
    const cancelRun = async (run: Run) => { try {
        await api(`/runs/${run.id}/cancel`, 'POST', {});
        await refreshRuns();
        notify(run.mode==='codex'?'已记录取消；已开始的原生生成可能仍完成并消耗 Codex 额度':'已记录取消操作，已提交的任务可能仍产生结果和费用');
    }
    catch (e) {
        notify((e as Error).message);
    } };
    const exportManual = (run: Run) => { downloadText(`canvepp-${run.id}.txt`, [`任务：${run.id}`, `模式：手动，外部结果由用户确认`, `提示词：\n${run.inputSnapshot.prompt}`, `尺寸：${run.inputSnapshot.params.size}`, `质量：${run.inputSnapshot.params.quality}`, `张数：${run.inputSnapshot.params.n}`, `参考图版本（按顺序）：${snapshotSources(run.inputSnapshot).join(', ') || '无'}`].join('\n\n')); };
    const attachResults = async () => { if (!attachRun)
        return; setRunBusy(true); try {
        await api<Run>(`/runs/${attachRun.id}/attach`, 'POST', { assetVersionIds: attachAssets, sourceNote });
        setAttachRun(undefined);
        await refreshRuns();
        notify('结果已关联任务，来源标记为用户确认');
    }
    catch (e) {
        notify((e as Error).message);
    }
    finally {
        setRunBusy(false);
    } };
    const viewerAsset = assets.find(a => a.id === viewer) ?? (libraryPreview?.id === viewer ? libraryPreview : undefined);
    const libraryItems = useMemo(() => localLibrary(assets, runs, graph, detail?.project.name || ''), [assets, runs, graph, detail?.project.name]);
    const locateLibraryImage = async (item: LibraryImage) => {
        if (loading || projectBusy || uploading || runBusy) return;
        if (item.asset.projectId !== detailRef.current?.project.id) await switchCanvas(item.asset.projectId);
        if (detailRef.current?.project.id !== item.asset.projectId) return;
        const node = graphRef.current.nodes.find(node => node.id === item.nodeId);
        setActiveId(node?.id); setSelectedIds(node ? [node.id] : []); setSelectedEdges([]);
        setVersionsOpen(!!node); setHistoryView('images'); openHistory();
        if (node) requestAnimationFrame(() => { void flow.fitView({ nodes: [{ id: node.id }], padding: 1, maxZoom: 1, duration: 250 }); });
        else notify('原卡片已移除，图片仍保留，可从版本中继续编辑。');
    };
    const compareAssets = compareIds.map(aid => assets.find(a => a.id === aid)).filter(Boolean) as AssetVersion[];
    const runCount = runs.filter(r => !r.historyOnly && (PENDING.includes(r.status)||r.status==='unknown'||r.status==='awaiting_import')).length;
    const editorInputs = active ? getEditorInputs(graph, active) : undefined;
    const activeLatestRun = active ? runs.find(run=>run.nodeId===active.id) : undefined;
    const selectedPictures = selectedIds.map(id=>graph.nodes.find(node=>node.id===id)).filter(node=>node && isImageCreator(node.kind)) as CanvasNode[];
    const multiSelection = selectedIds.length>1;
    const multiDisabledReason = selectedPictures.length!==selectedIds.length ? '请选择图片卡片作为参考' : runs.some(run=>!run.historyOnly && generationGroup(run)?.sourceNodeIds.some(id=>selectedIds.includes(id))&&[...PENDING,'unknown'].includes(run.status)) ? '这一组图片已有待处理的生成任务' : undefined;
    const independentInput = (port: 'prompt' | 'params') => {
        if (!active) return;
        updateGraph(current => makeInputIndependent(current, active.id, port));
        notify(`已复制${port === 'prompt' ? '提示词' : '参数'}并切换为独立编辑`);
    };
    const runOutput = selectedRun && assets.find(asset => selectedRun.assetVersionIds.includes(asset.id));
    const runDetails = selectedRun ? <>
    <div className="preview-prompt">{selectedRun.inputSnapshot.prompt}</div>
    <div className="run-size-line">目标 {selectedRun.inputSnapshot.params.size.replace('x',' × ')} px{runOutput && <> · 实际 {runOutput.width} × {runOutput.height} px</>} · {MODE_NAMES[selectedRun.mode]}</div>
    {!selectedRun.historyOnly && selectedRun.mode==='codex' && !!selectedRun.usage?.canvasExecution && <div className="history-run-actions">
        {PENDING.includes(selectedRun.status) && <button className="text-button small" onClick={()=>cancelRun(selectedRun)}><Square size={11}/>取消生成</button>}
        {selectedRun.status==='unknown' && <button className="text-button small" disabled={runBusy} onClick={async()=>{
            setRunBusy(true);
            try { await api(`/runs/${selectedRun.id}/${selectedRun.errorClass==='storage_error'?'recover':'check-result'}`,'POST',{}); await refreshRuns(); notify('正在核查原执行记录'); }
            catch(error){notify((error as Error).message);} finally{setRunBusy(false);}
        }}>核查已有结果</button>}
    </div>}
    {!selectedRun.historyOnly && selectedRun.mode==='codex' && !selectedRun.usage?.canvasExecution && [...PENDING,'unknown'].includes(selectedRun.status)&&<section className="native-handoff-panel" aria-label="旧任务交接">
    <div className="native-handoff-heading"><Clock3 size={17}/><strong>{selectedRun.status==='awaiting_host'?(hostDispatchStatus(selectedRun,hostDispatches[selectedRun.id])==='unknown'?'交接结果待确认':hostDispatchStatus(selectedRun,hostDispatches[selectedRun.id])?'已交接，等待 Codex':'等待交给 Codex'):selectedRun.status==='unknown'?'请检查当前 Codex 对话':selectedRun.status==='running'?'Codex 正在处理':runStatusName(selectedRun)}</strong></div>
    {selectedRun.status==='awaiting_host'?<p>{hostDispatchStatus(selectedRun,hostDispatches[selectedRun.id])==='unknown'?'未收到确定的交接回执。请检查当前 Codex 对话与任务 ID，确认已有请求；画布不会自动重新发送或生成。':hostDispatchStatus(selectedRun,hostDispatches[selectedRun.id])?'请求已排入绑定的 Codex 对话，尚未开始生成。任务被宿主领取后状态才会更新。':hostContext.connected?'已连接当前 Codex 对话。点击“交给 Codex”后排队处理；这一步不会自动采用输出。':'请从 Codex 打开画布以连接当前对话。当前只保存了已确认任务；可复制交接指令粘贴到 Codex 对话继续处理。'}</p>:selectedRun.status==='unknown'?<p>原生工具的执行或回填结果尚未确认。请检查当前 Codex 对话中的任务 ID 与工具结果；不要重复生图。这里不使用 API 美元费用对账。</p>:selectedRun.status==='running'?<p>任务已被 Codex 宿主领取。生成消耗 Codex 账户额度；完成后结果会保存为独立版本，需手动采用。</p>:<p>本任务使用 Codex 账户额度。尺寸为生成意图，实际像素以回填为准；保存的结果需要手动采用。</p>}
    <div className="native-handoff-actions">
      {selectedRun.status==='awaiting_host'&&hostContext.connected&&!hostDispatchStatus(selectedRun,hostDispatches[selectedRun.id])&&<button className="button primary compact" disabled={hostDispatchBusy===selectedRun.id} onClick={()=>dispatchCodexRun(selectedRun)}>{hostDispatchBusy===selectedRun.id?<Loader2 size={13} className="spin"/>:<ArrowUpRight size={13}/>}交给 Codex</button>}
      <button className="button secondary compact" onClick={()=>copyCodexHandoff(selectedRun)}><Copy size={13}/>{selectedRun.status!=='awaiting_host'||hostDispatchStatus(selectedRun,hostDispatches[selectedRun.id])?'复制任务检查指令':'复制交接指令'}</button>
      {PENDING.includes(selectedRun.status)&&<button className="text-button small" disabled={hostDispatchBusy===selectedRun.id} onClick={()=>cancelRun(selectedRun)}><Square size={11}/>{selectedRun.status==='awaiting_host'?'取消待处理任务':'记录取消'}</button>}
    </div>
    {selectedRun.status==='unknown'&&selectedRun.errorClass==='storage_error'&&<button className="button secondary full" disabled={runBusy} onClick={async()=>{setRunBusy(true);try{const recovered=await api<Run>(`/runs/${selectedRun.id}/recover`,'POST',{});setShowRun(recovered);await refreshRuns();notify('已尝试恢复本地暂存结果，没有重新生成');}catch(error){notify((error as Error).message);}finally{setRunBusy(false);}}}><ArrowDownToLine size={13}/>恢复本地暂存结果 · 不生成</button>}
  </section>}{typeof selectedRun.usage?.sourceNote==='string'&&<p className="dialog-copy">用户确认的来源：{selectedRun.usage.sourceNote}</p>}<details className="history-task-meta"><summary>状态变化<ChevronDown size={12}/></summary><div className="run-events">{selectedRun.events.map(event => <div key={event.eventId}><span className="event-dot"/><time>{fmtTime(event.occurredAt)}</time><span>{selectedRun.mode==='codex'&&event.state==='unknown'?'状态待确认':STATUS_NAMES[event.state]}</span><code>#{event.sequence}</code></div>)}</div></details>{selectedRun.error && <div className="notice warning"><AlertCircle size={14}/><span>{selectedRun.error}</span></div>}{selectedRun.mode!=='codex'&&selectedRun.status === 'unknown' && <><div className="notice warning"><ShieldCheck size={15}/><span>受理状态尚不确定。本工具保留预算，重开与刷新不会重新提交。请先在供应商侧确认结果。</span></div>{selectedRun.errorClass === 'storage_error' && <button className="button secondary full" disabled={runBusy} onClick={async () => { setRunBusy(true); try {
        const recovered = await api<Run>(`/runs/${selectedRun.id}/recover`, 'POST', {});
        setShowRun(recovered);
        await refreshRuns();
        notify('已尝试恢复暂存结果，没有重新生成');
    }
    catch (e) {
        notify((e as Error).message);
    }
    finally {
        setRunBusy(false);
    } }}><ArrowDownToLine size={14}/>从本地暂存恢复结果 · 不生成</button>}<label className="field-label">人工对账结论</label><select className="form-select" value={reconcileOutcome} onChange={e => setReconcileOutcome(e.target.value as typeof reconcileOutcome)}><option value="not_accepted">已核实：供应商未受理，释放预留</option><option value="charged_no_output">已核实：已产生费用但没有结果，按上界结算</option></select><label className="field-label" style={{ marginTop: 15 }}>对账依据</label><textarea className="note-editor" aria-label="对账依据" value={reconcileNote} placeholder="记录供应商账单、请求查询或人工核实依据" onChange={e => setReconcileNote(e.target.value)}/><label className="checkbox-label"><input type="checkbox" checked={reconcileConfirm} onChange={e => setReconcileConfirm(e.target.checked)}/><span>我已核实上述结论，此操作只记录对账，不会重新生成。</span></label><button className="button primary full" disabled={!reconcileConfirm || !reconcileNote.trim() || runBusy} onClick={async () => { setRunBusy(true); try {
        const reconciled = await api<Run>(`/runs/${selectedRun.id}/reconcile`, 'POST', { outcome: reconcileOutcome, note: reconcileNote });
        setShowRun(reconciled);
        await refreshRuns();
        notify('对账结论已记录');
    }
    catch (e) {
        notify((e as Error).message);
    }
    finally {
        setRunBusy(false);
    } }}>{runBusy ? <Loader2 size={14} className="spin"/> : <ShieldCheck size={14}/>}确认记录对账</button></>}<div className="history-run-actions">{selectedRun.status==='awaiting_import'&&<button className="button primary compact" onClick={()=>{setAttachRun(selectedRun);setAttachAssets([]);setAttachConfirm(false);setSourceNote('');}}><Upload size={13}/>回填结果</button>}{selectedRun.mode!=='codex'&&[...ACTIVE,'awaiting_import'].includes(selectedRun.status)&&<button className="text-button small" onClick={()=>cancelRun(selectedRun)}><Square size={11}/>取消任务</button>}{['failed','cancelled'].includes(selectedRun.status)&&graph.nodes.some(node=>node.id===selectedRun.nodeId)&&<button className="text-button small" onClick={()=>prepareRun(selectedRun.nodeId)}>重新预览<ArrowRight size={13}/></button>}</div></> : null;
    return <div className="app-shell">
  <header className="topbar compact-topbar">
    <div className="brand compact-brand" title="Canvepp"><span className="brand-mark"><span/><span/><span/></span></div>
    <input className="project-title-input" value={detail?.project.name || ''} disabled={!detail || loading || projectBusy} placeholder="画布名称" aria-label="画布名称" maxLength={120} onChange={e=>renameProject(e.target.value)} onBlur={()=>{if(detail)save().catch(e=>notify(e.message));}}/>
    <div className="canvas-picker-wrap" onKeyDown={e=>{if(e.key==='Escape')setCanvasPickerOpen(false);}}>
      <button className="toolbar-action" aria-label="切换画布" title="切换画布" aria-expanded={canvasPickerOpen} onClick={toggleCanvasPicker}><LayoutGrid size={15}/><span>画布</span><ChevronDown size={11}/></button>
      {canvasPickerOpen&&<><button className="menu-dismiss" aria-label="关闭画布菜单" onClick={()=>setCanvasPickerOpen(false)}/><div className="project-menu canvas-picker-menu">
        <button disabled={loading||projectBusy||uploading||runBusy} onClick={()=>void createCanvas()}><Plus size={15}/><span>新建画布</span></button>
        {projects.length>0&&<div className="menu-divider"/>}
        {projects.map(p=><button key={p.id} aria-current={p.id===detail?.project.id?'page':undefined} disabled={loading||projectBusy||uploading||runBusy} onClick={()=>void switchCanvas(p.id)}><span className="canvas-picker-name"><strong>{p.name}</strong><small>{p.graph.nodes.length} 张卡片</small></span>{p.id===detail?.project.id&&<Check size={14}/>}</button>)}
      </div></>}
    </div>
    <span className={`save-indicator save-${saveState}`} title={{saved:'已保存到本地',saving:'正在保存',unsaved:'等待保存',conflict:'同时修改的内容待选择',error:'保存失败，草稿已保留'}[saveState]}>{saveState==='saved'?<CheckCheck size={13}/>:saveState==='saving'?<Loader2 size={13} className="spin"/>:<Circle size={8}/>}<span>{{saved:'已保存',saving:'保存中',unsaved:'待保存',conflict:'待合并',error:'保存失败'}[saveState]}</span></span>
    <span className="topbar-image-count" title={`${graph.nodes.length} 张图片卡片`}><Images size={14}/><span>{graph.nodes.length}</span></span>
    <div className={`topbar-mode mode-${mode}`} title={mode==='codex'?'Codex 原生 · 消耗 Codex 额度':MODE_NAMES[mode]}><Sparkles size={14}/><select aria-label="运行模式" value={mode} onChange={e=>setMode(e.target.value as RunMode)}><option value="codex">Codex</option><option value="mock">模拟</option><option value="manual">手动</option><option value="api">Images API</option></select><ChevronDown size={11}/></div>
    <div className="topbar-right">
      {saveState==='error'&&<button className="text-button small" onClick={()=>save().catch(e=>notify(e.message))}>重试保存</button>}
      <div className="backup-menu-wrap">
        <button className={`toolbar-action ${projectMenu?'is-active':''}`} aria-label="备份菜单" aria-expanded={projectMenu} onClick={()=>setProjectMenu(!projectMenu)}><FolderOpen size={16}/><span>备份</span><ChevronDown size={11}/></button>
        {projectMenu&&<><button className="menu-dismiss" aria-label="关闭备份菜单" onClick={()=>setProjectMenu(false)}/><div className="project-menu backup-menu">
          <button disabled={!detail||loading||projectBusy} onClick={()=>{setProjectMenu(false);void exportProject();}}><Download size={15}/><span>导出画布备份</span></button>
          <button disabled={!detail||loading||projectBusy} onClick={()=>{setProjectMenu(false);setRestoreOpen(true);}}><Upload size={15}/><span>从备份恢复</span></button>
        </div></>}
      </div>
      <button className={`icon-button panel-toggle ${panelOpen?'is-active':''}`} title="图片设置" aria-label={panelOpen?'收起图片设置':'打开图片设置'} onClick={()=>panelOpen?setPanelOpen(false):openInspector()}><svg width="19" height="19" viewBox="0 0 24 24" fill="none" aria-hidden="true"><rect x="3" y="4" width="18" height="16" rx="3" stroke="currentColor" strokeWidth="1.5"/><path d="M15 4v16" stroke="currentColor" strokeWidth="1.5"/><path d="M15 5h3a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2h-3z" fill="currentColor" opacity={panelOpen?'.2':'.06'}/><path d={panelOpen?'m8 9 3 3-3 3':'m11 9-3 3 3 3'} stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"/></svg></button>
    </div>
  </header>
  {mergeReview && <Modal title="这几项内容在两个窗口同时修改了" eyebrow="其他修改已自动合并" onClose={() => {}} dismissible={false}>
    <p className="muted">本窗口草稿已保留。选择如何处理下面的同时修改，随后继续自动保存。</p>
    <div className="merge-conflicts">{mergeReview.conflicts.map(conflict => <section key={conflict.path} className="merge-conflict">
      <h3>{conflict.label}</h3><div className="merge-values"><div><strong>本窗口</strong><pre>{formatMergeValue(conflict.local)}</pre></div><div><strong>另一窗口</strong><pre>{formatMergeValue(conflict.remote)}</pre></div></div>
    </section>)}</div>
    <div className="merge-actions"><button className="button secondary" onClick={() => resolveMerge('remote')}>采用另一窗口的冲突内容</button><button className="button primary" onClick={() => resolveMerge('local')}>保留本窗口的冲突内容</button></div>
  </Modal>}
  <div className="workspace-body">
   <aside className="tool-rail" aria-label="画布工具">
    <button className="rail-tool primary-image-tool" disabled={!detail||loading||projectBusy} onClick={()=>addNode('generate')} title="新建图片" aria-label="新建图片"><Plus size={21}/><span>新建</span></button>
    <button className="rail-tool" disabled={!detail||loading||projectBusy} onClick={()=>addNode('image')} title="导入图片" aria-label="导入图片"><ImagePlus size={20}/><span>导入</span></button>
    <div className="rail-divider"/>
    <button className={`rail-tool ${historyOpen?'is-active':''}`} aria-expanded={historyOpen} title="历史" aria-label="历史" onClick={()=>historyOpen?setHistoryOpen(false):openRunHistory()}><History size={20}/><span>历史</span>{runCount>0&&<b className="rail-badge">{runCount}</b>}</button>
    <button className={`rail-tool ${trashOpen?'is-active':''}`} aria-expanded={trashOpen} title="回收站" aria-label="回收站" onClick={()=>trashOpen?setTrashOpen(false):openTrash()}><Trash2 size={20}/><span>回收站</span></button>
    <div className="rail-spacer"/><button className="rail-tool rail-help" onClick={()=>setHelpOpen(true)} title="使用指南" aria-label="使用指南"><CircleHelp size={18}/></button>
   </aside>
   {historyOpen&&<aside className="image-history-panel" aria-label="历史侧栏" inert={loading||projectBusy}>
     <ImageLibrary view={historyView} onViewChange={setHistoryView} activity={<RunHistory key={projectId} runs={runs} nodes={graph.nodes} assets={assets} selectedId={selectedRun?.id} onSelect={run=>{setShowRun(run);setReconcileNote('');setReconcileConfirm(false);setReconcileOutcome('not_accepted');}} onView={setViewer} onLocate={group=>{
         const ids=group.map(run=>{
             const result = graph.nodes.find(node => node.id === run.nodeId);
             if (result?.selectedAssetVersionId && run.assetVersionIds.includes(result.selectedAssetVersionId)) return result.id;
             return graph.nodes.find(node => node.selectedAssetVersionId && run.assetVersionIds.includes(node.selectedAssetVersionId))?.id ?? result?.id;
         }).filter((id):id is string=>!!id);
         if(ids.length){setActiveId(ids[0]);setSelectedIds(ids);setSelectedEdges([]);void flow.fitView({nodes:ids.map(id=>({id})),padding:.4,maxZoom:1,duration:240});}
     }} status={runStatusName}>{runDetails}</RunHistory>} items={libraryItems} projectId={detail?.project.id || ''} activeAssetId={activeAssetId} activeName={active?.name} versionCount={nodeAssets.length} versionsOpen={versionsOpen} onVersionsOpen={setVersionsOpen} onClose={()=>setHistoryOpen(false)} onView={asset=>{setLibraryPreview(asset);setViewer(asset.id);}} onLocate={locateLibraryImage} fetchPage={fetchLibraryPage}>
<section className="history-section"><div className="history-intro">{compareIds.length > 0 && <button className="button secondary compact" disabled={compareIds.length !== 2} onClick={() => setCompareOpen(true)}><Images size={13}/>对比 {compareIds.length}/2</button>}</div>{!nodeAssets.length ? <div className="panel-empty"><History size={28} strokeWidth={1.2}/><strong>暂无版本记录</strong><p>导入图片或运行后，在这里查看版本和来源。</p></div> : nodeAssets.map((asset, index) => { const run = runs.find(r => r.id === asset.runId); return <article className={`version-card ${asset.id === activeAssetId ? 'is-current' : ''}`} key={asset.id}><button className="version-thumbnail" onClick={() => setViewer(asset.id)}><img src={thumbnailUrl(asset)} alt={`版本 ${nodeAssets.length - index}`}/><span>{asset.source === 'codex' ? 'Codex' : asset.source === 'mock' ? '模拟' : asset.source === 'manual' ? '手动' : 'v' + (nodeAssets.length - index)}</span></button><div className="version-content"><div className="version-title"><strong>{asset.id === referenceAssetId ? '参考原图' : `版本 ${nodeAssets.length - index}`}</strong><button className={asset.favorite ? 'favorite active' : 'favorite'} title="收藏版本" aria-label={`收藏版本 ${nodeAssets.length - index}`} onClick={() => updateAsset(asset, { favorite: !asset.favorite })}><Star size={14} fill={asset.favorite ? 'currentColor' : 'none'}/></button></div><div className="version-sub">{fmtTime(asset.createdAt)} · {asset.width} × {asset.height}</div>{asset.id === activeAssetId ? <span className="current-badge"><Check size={10}/>当前采用</span> : active ? <button className="adopt-button" onClick={() => chooseAsset(asset)}>采用此版本</button> : <span className="version-sub">已保留</span>}</div><div className="version-footer"><label><input type="checkbox" aria-label={`加入对比版本 ${nodeAssets.length - index}`} checked={compareIds.includes(asset.id)} disabled={!compareIds.includes(asset.id) && compareIds.length >= 2} onChange={e => setCompareIds(old => e.target.checked ? [...old, asset.id] : old.filter(x => x !== asset.id))}/>加入对比</label><button onClick={() => branchFrom(asset)}><GitBranch size={12}/>继续编辑</button></div>{run && <details className="version-provenance"><summary>生成提示词 <ChevronDown size={11}/></summary><p>{run.inputSnapshot.prompt}</p>{typeof run.usage?.sourceNote==='string'&&<p>用户确认的来源：{run.usage.sourceNote}</p>}{run.draftChanged && <span>成功，但输入已变更</span>}</details>}</article>; })}</section>
     </ImageLibrary>
   </aside>}
   {trashOpen&&<aside className="image-history-panel trash-panel" aria-label="回收站侧栏" inert={loading||projectBusy}><TrashPanel items={trash} busy={trashBusy} error={trashError} onClose={()=>setTrashOpen(false)} onRestore={ids=>void restoreTrash(ids)} onView={setViewer} onRefresh={()=>setTrashRefresh(value=>value+1)}/></aside>}
   <main className={`canvas-area ${dropActive?'is-file-drop':''}`} tabIndex={0} ref={workspaceRef} onKeyDown={keyboard} onPaste={pasteEvent} onContextMenu={showContextMenu} aria-label="节点画布" onDragEnter={event=>{if(event.dataTransfer.types.some(type=>['Files','text/uri-list','text/html'].includes(type))){event.preventDefault();setDropActive(true);}}} onDragLeave={event=>{if(!event.currentTarget.contains(event.relatedTarget as Node))setDropActive(false);}} onDragOver={event=>{event.preventDefault();event.dataTransfer.dropEffect='copy';}} onDrop={event=>{event.preventDefault();setDropActive(false);void importTransfer(event.dataTransfer,flow.screenToFlowPosition({x:event.clientX,y:event.clientY}));}}>
    {dropActive&&<div className="canvas-drop-hint"><ImagePlus size={20}/>松开以插入图片</div>}
    <CanvasFlowSurface nodes={cards} edges={flowEdges} nodeTypes={nodeTypes} positionStore={positionStore} showMiniMap={showMiniMap} onNodesChange={onNodesChange} onEdgesChange={onEdgesChange} onConnect={onConnect} onConnectEnd={onConnectEnd} isValidConnection={isValidConnection} onNodeClick={onNodeClick} onPaneClick={onPaneClick} onEdgeClick={onEdgeClick} onSelectionChange={onSelectionChange} onNodeDragStart={startDrag} onNodeDragStop={finishDrag} onMoveEnd={onMoveEnd} defaultViewport={graph.viewport || DEFAULT_VIEWPORT} minZoom={.15} maxZoom={4} panOnDrag={PAN_BUTTONS} panActivationKeyCode="Space" selectionOnDrag selectionKeyCode="Shift" multiSelectionKeyCode={MULTI_SELECT_KEYS} deleteKeyCode={null} selectNodesOnDrag={false} nodesFocusable edgesFocusable zoomOnScroll proOptions={FLOW_PRO_OPTIONS}/>

    {!loading && !detail && <div className="canvas-empty"><AlertCircle size={30}/><h2>创建第一张画布</h2><button className="button primary" disabled={projectBusy} onClick={()=>void createCanvas()}><Plus size={15}/>新建画布</button></div>}{!loading && detail && !graph.nodes.length && <div className="canvas-empty"><div className="empty-art"><span /><span /><span /><Link2 size={23}/></div><h2>从一张图片开始</h2><p>新建或导入图片，在右侧写下想法，再预览确认。</p><button className="button primary" onClick={() => addNode('generate')}><Plus size={16}/>新建图片</button><button className="text-button" onClick={() => addNode('image')}><ImagePlus size={15}/>导入图片继续编辑</button></div>}
    {(loading||projectBusy) && <div className="loading-overlay"><Loader2 size={26} className="spin"/><span>正在打开画布…</span></div>}
    <div className="canvas-bottom-toolbar"><div className="history-controls"><button aria-label="撤销" title="撤销 ⌘Z" disabled={!undoCount} onClick={undo}><Undo2 size={16}/></button><button aria-label="重做" title="重做 ⇧⌘Z" disabled={!redoCount} onClick={redo}><Redo2 size={16}/></button><span /><button aria-label="框选工具" title="拖动框选，空格平移" onClick={() => { workspaceRef.current?.focus(); notify('拖动空白区域框选；按住空格拖动平移画布'); }}><MousePointer2 size={15}/></button></div><div className="canvas-hint">点击图片编辑 <span>·</span> <kbd>空格</kbd> 拖动平移 <span>·</span> 滚轮缩放</div><div className="zoom-controls"><button aria-label="缩小" onClick={() => flow.zoomOut({ duration: 150 })}><ZoomOut size={15}/></button><select aria-label="画布缩放" value={zoom} onChange={event=>flow.zoomTo(Number(event.target.value)/100,{duration:150})}>{![25,50,100,150,200,300,400].includes(zoom)&&<option value={zoom}>{zoom}%</option>}{[25,50,100,150,200,300,400].map(value=><option key={value} value={value}>{value}%</option>)}</select><button aria-label="放大" onClick={() => flow.zoomIn({ duration: 150 })}><ZoomIn size={15}/></button><i /><button aria-label="适应画布" title="适应画布" onClick={() => flow.fitView({ ...FIT_VIEW_OPTIONS, duration: 300 })}><Maximize2 size={15}/></button><button className={showMiniMap ? 'active' : ''} aria-label="切换小地图" title="小地图" onClick={() => setShowMiniMap(!showMiniMap)}><LayoutGrid size={15}/></button></div></div>
   </main>
   {panelOpen && <aside className="inspector" aria-label="图片设置侧栏" inert={loading||projectBusy}><div className="drawer-heading"><strong>{multiSelection?`多图参考 · ${selectedPictures.length} 张`:'图片设置'}</strong><button className="icon-button" aria-label="关闭图片设置" onClick={()=>setPanelOpen(false)}><X size={15}/></button></div>
    {multiSelection ? <MultiReferenceSettings key={`${projectId}:${[...selectedIds].sort().join(',')}`} projectId={projectId||''} nodes={selectedPictures} assets={assets} mode={mode} busy={previewBusy||runBusy} disabledReason={multiDisabledReason} onGenerate={prepareGroup} onView={setViewer}/> : active ? <>{!isImageCreator(active.kind) && active.kind !== 'output' && <><div className="inspector-node-title"><span className={`inspector-kind-icon ${active.kind}`}>{React.createElement(ICONS[active.kind], { size: 19 })}</span><div><span className="small-eyebrow">{isImageCreator(active.kind) ? active.kind === 'image' ? '编辑这张图' : '图片创作' : `${NODE_LABELS[active.kind]}节点`}</span><input aria-label="节点名称" maxLength={200} value={active.name} onChange={e => changeNode(active.id, { name: e.target.value })}/></div><button className="icon-button tiny" title="复制此节点" aria-label="复制此节点" onClick={duplicateActive}><Copy size={15}/></button></div>
</>}
    <div className="inspector-scroll">
     {active.kind === 'text' && <section className="property-section"><label className="field-label">共享提示词 <span>{active.config.text?.length || 0} 字</span></label><textarea className="prompt-editor" aria-label="共享提示词" maxLength={32000} placeholder="写下多个图片可复用的提示词…" value={active.config.text || ''} onChange={e => changeNode(active.id, { config: { ...active.config, text: e.target.value } })}/><p className="field-help">这是旧画布的共享输入，修改会影响仍与它关联的图片。</p></section>}
     {isImageCreator(active.kind) && editorInputs && <section className="property-section image-prompt-section">
      <label className="field-label" htmlFor="image-prompt">{active.kind === 'image' || active.kind === 'edit' ? '提示词 · 修改要求' : '提示词'}<span>{editorInputs.prompt.length} 字</span></label>
      <textarea id="image-prompt" className="prompt-editor image-prompt-editor" aria-label="图片提示词" maxLength={32000} readOnly={!!editorInputs.promptEdge} placeholder={active.kind === 'image' || active.kind === 'edit' ? '描述你希望怎样修改这张图片…' : '描述你想要的画面、主体与风格…'} value={editorInputs.prompt} onChange={event=>changeNode(active.id,{config:{...active.config,text:event.target.value}})}/>
      {editorInputs.promptEdge ? <div className="connected-input-source"><button onClick={()=>{if(editorInputs.promptSource){setActiveId(editorInputs.promptSource.id);setSelectedIds([editorInputs.promptSource.id]);}}}><Link2 size={12}/><span>来自 {editorInputs.promptSource?.name || '未知来源'}</span><ArrowUpRight size={11}/></button><button onClick={()=>independentInput('prompt')}>独立编辑</button></div> : null}
     </section>}
     {['params','image','generate','edit'].includes(active.kind) && editorInputs && <section className="property-section image-size-section">
      {editorInputs.paramsEdge && <div className="connected-input-source parameters-source"><button onClick={()=>{if(editorInputs.paramsSource){setActiveId(editorInputs.paramsSource.id);setSelectedIds([editorInputs.paramsSource.id]);}}}><Link2 size={12}/><span>尺寸来自 {editorInputs.paramsSource?.name || '共享参数'}</span><ArrowUpRight size={11}/></button><button onClick={()=>independentInput('params')}>独立编辑</button></div>}
      <ImageControls key={`${active.id}:${editorInputs.paramsEdge?.id || 'own'}:${mode}`} nodeId={active.id} params={editorInputs.params} mode={mode} connected={!!editorInputs.paramsEdge} onValidity={setSizeValidity} onChange={params=>changeNode(active.id,{config:{...active.config,params}})} onSingleImage={()=>{
          const nodeId=active.id;
          updateGraph(current=>{
              const independent=makeInputIndependent(current,nodeId,'params');
              return {...independent,nodes:independent.nodes.map(node=>node.id===nodeId?{...node,config:{...node.config,params:{...getEditorInputs(independent,node).params,n:1}}}:node)};
          });
          notify(editorInputs.paramsEdge?'已复制参数为独立配置并设为 1 张，共享节点保持不变':'已将本图配置改为 1 张');
      }}/>
     </section>}
     {activeAsset && <section className="property-section image-quick-actions"><button className="button secondary compact" onClick={()=>downloadAsset(activeAsset)}><Download size={13}/>导出</button><button className="button secondary compact" onClick={()=>branchFrom(activeAsset)}><GitBranch size={13}/>继续编辑</button><button className="icon-button" title="查看大图" aria-label="查看大图" onClick={()=>setViewer(activeAsset.id)}><Expand size={15}/></button></section>}
     {(isImageCreator(active.kind)||active.kind==='output') && <details className="property-section image-details" key={`image-details-${active.id}`}><summary>图片详情<ChevronDown size={12}/></summary>
      <label className="field-label">名称</label><div className="image-name-field"><input className="form-input" aria-label="节点名称" maxLength={200} value={active.name} onChange={event=>changeNode(active.id,{name:event.target.value})}/><button className="icon-button" title="复制此节点" aria-label="复制此节点" onClick={duplicateActive}><Copy size={14}/></button></div>
      {activeAsset&&<><button className="image-detail-preview" onClick={()=>setViewer(activeAsset.id)}><img src={thumbnailUrl(activeAsset)} alt="当前采用的图片"/></button><div className="image-file-summary"><strong>{activeAsset.name}</strong><span>{activeAsset.width} × {activeAsset.height} · {activeAsset.mime.split('/')[1].toUpperCase()} · {(activeAsset.byteSize/1024).toFixed(0)} KB</span><span>{activeAsset.source==='codex'?'Codex 原生输出':activeAsset.source==='mock'?'模拟输出':activeAsset.source==='manual'?'手动回填':activeAsset.source==='api'?'API 输出':'导入图片'} · {fmtTime(activeAsset.createdAt)}</span></div><div className="current-version-line"><Check size={12}/><span>当前采用的版本</span><button className="inline-link" onClick={()=>{setVersionsOpen(true);setHistoryView('images');openHistory();}}>版本历史<ChevronRight size={12}/></button></div></>}
      {active.kind==='image'&&<><button className="button secondary full" disabled={uploading} onClick={()=>requestUpload(active.id)}><Upload size={14}/>{activeAsset?'导入另一个图片版本':'导入图片'}</button><label className="field-label import-strategy-label" htmlFor="metadata-policy">导入保存策略</label><select id="metadata-policy" className="form-select" value={metadataPolicy} onChange={event=>{const policy=event.target.value as 'strip'|'preserve';setMetadataPolicy(policy);localStorage.setItem('canvepp:metadata-policy',policy);}}><option value="strip">去除 EXIF，保存净化副本</option><option value="preserve">原样保存原始文件</option></select><p className="field-help">{metadataPolicy==='strip'?'去除 EXIF 后保存净化副本，不另存原始字节。':'保留原文件字节，可能包含拍摄地点等 EXIF 信息。'}导入会创建新版本，不覆盖历史。</p></>}
     </details>}
     {(active.kind === 'generate' || active.kind === 'edit') && graph.edges.some(edge=>edge.targetNodeId===active.id) && <details className="property-section advanced-input-connections"><summary>已有图片关联<span>{graph.edges.filter(edge=>edge.targetNodeId===active.id).length} 条</span><ChevronDown size={12}/></summary>{graph.edges.filter(edge=>edge.targetNodeId===active.id).map(edge=>{const source=graph.nodes.find(node=>node.id===edge.sourceNodeId);return <div className="input-reference" key={edge.id}><span>{PORT_LABEL[edge.targetPort]}</span>{source&&<button onClick={()=>{setActiveId(source.id);setSelectedIds([source.id]);}}><Link2 size={11}/>{source.name}<ArrowUpRight size={11}/></button>}</div>;})}</details>}
     {active.kind === 'output' && !activeAsset && <section className="property-section"><div className="notice"><Link2 size={15}/><span>此旧输出暂无图片。可直接选中图片查看或导出。</span></div></section>}
     {isImageCreator(active.kind) && !!runInputs[active.id]?.snapshot?.upstreamStale.length && <div className="property-section upstream-stale-note"><History size={13}/><span>上游图片的输入已更新；预览时请确认是否继续使用当前选定的旧版本。</span></div>}
     {activeLatestRun&&['failed','unknown','partial'].includes(activeLatestRun.status)&&<section className="property-section current-run-issue"><button onClick={()=>{setShowRun(activeLatestRun);}}><AlertCircle size={14}/><span>{runStatusName(activeLatestRun)} · 查看任务详情</span><ChevronRight size={12}/></button></section>}
     <details className="property-section editor-extra-details" key={`notes-${active.id}`}><summary>创作备注<ChevronDown size={12}/></summary><textarea className="note-editor" aria-label="创作备注" maxLength={4000} placeholder="记录这张图片的想法，仅保存在本地…" value={active.config.note || ''} onChange={event=>changeNode(active.id,{config:{...active.config,note:event.target.value}})}/></details>
     <div className="editor-delete-row"><button onClick={()=>setDeleteTarget({ nodeIds: [active.id], edgeIds: [] })}><Trash2 size={12}/>删除{isImageCreator(active.kind)?'图片':'节点'}</button></div>

    </div>
    {isImageCreator(active.kind) && <div className="inspector-run-footer"><button className="button primary full run-primary" title={runReason(active) || '生成图片'} disabled={!!runReason(active) || previewBusy || runs.some(r => r.nodeId === active.id && PENDING.includes(r.status))} onClick={() => prepareRun(active.id)}>{previewBusy ? <Loader2 size={16} className="spin"/> : <Play size={14} fill="currentColor"/>}<span>{mode === 'manual' ? `创建 ${editorInputs?.params.n||1} 张任务` : `生成 ${editorInputs?.params.n||1} 张图片`}</span><kbd>↵</kbd></button></div>}</> : <div className="inspector-unselected"><div className="unselected-icon"><MousePointer2 size={25} strokeWidth={1.3}/></div><h3>选择一张图片</h3><p>点击图片，在这里填写提示词、调整参数，<br />在左侧历史里比较已有版本。</p><div className="quick-guide"><span><i>1</i>新建图片，或导入一张图片</span><span><i>2</i>在侧栏写下想法或修改要求</span><span><i>3</i>预览确认，再开始创作</span></div><button className="inline-link" onClick={() => setHelpOpen(true)}>了解画布操作<ArrowUpRight size={13}/></button></div>}
   </aside>}
  </div>

  {restoreOpen&&<Modal title="从备份恢复" onClose={()=>!projectBusy&&setRestoreOpen(false)} className="small-dialog"><p className="dialog-copy">选择 .canvepp.zip 备份，图片、卡片和历史会加入当前画布，现有内容完整保留。</p><p className="dialog-copy muted">恢复后，可用撤销移除新加入的卡片；原图和版本仍会保留在历史中。</p><div className="modal-actions"><button className="button secondary" disabled={projectBusy} onClick={()=>setRestoreOpen(false)}>取消</button><button className="button primary" disabled={projectBusy} onClick={()=>packageInput.current?.click()}>{projectBusy?<Loader2 size={15} className="spin"/>:<Upload size={15}/>}选择备份文件</button></div></Modal>}
  <input ref={fileInput} className="hidden-input" type="file" accept="image/png,image/jpeg,image/webp" multiple={!!attachRun} onChange={e => importFiles(e.target.files)}/><input ref={packageInput} className="hidden-input" type="file" accept=".zip" onChange={e => importProject(e.target.files?.[0])}/>
  {(toast || uploading) && <div className="toast" role="status">{uploading ? <Loader2 size={15} className="spin"/> : <Check size={15}/>}<span>{uploading ? '正在导入图片…' : toast}</span><button aria-label="关闭提示" onClick={() => setToast('')}><X size={13}/></button></div>}
  {groupPreview && <Modal title={groupPreview.references.length>1?`共同参考 ${groupPreview.references.length} 张，生成 ${groupPreview.params.n} 张`:`生成 ${groupPreview.params.n} 张独立图片`} onClose={()=>{if(!runBusy)setGroupPreview(undefined);}} dismissible={!runBusy} className="group-preview-dialog">
    {groupPreview.references.length>0&&<div className="group-preview-references">{groupPreview.references.map((asset,index)=><figure key={`${asset.id}:${index}`}><img src={thumbnailUrl(asset)} alt={`参考图 ${index+1}`}/><figcaption>参考图 {index+1}</figcaption></figure>)}</div>}
    <p className="group-preview-prompt">{groupPreview.prompt}</p>
    <div className="group-preview-summary"><span>{groupPreview.params.size.replace('x',' × ')} px</span><span>{groupPreview.params.n} 张独立图片</span><span>自动连接来源</span></div>
    {groupPreview.mode==='codex'&&<p className="muted">将依次进行 {groupPreview.params.n} 次 Codex 原生生图，消耗 Codex 额度。</p>}
    {groupPreview.mode==='mock'&&<p className="muted">模拟模式只生成本地测试图片，不调用模型。</p>}
    {(groupPreview.references.length>0 && groupPreview.snapshots.some(snapshot=>snapshot.upstreamStale.length))&&<label className="check-row"><input type="checkbox" checked={acceptStale} onChange={event=>setAcceptStale(event.target.checked)}/>继续使用当前选定的参考图版本</label>}
    <div className="modal-actions"><button className="button secondary" disabled={runBusy} onClick={()=>setGroupPreview(undefined)}>返回调整</button><button className="button primary" disabled={runBusy||(!acceptStale&&(groupPreview.references.length>0 && groupPreview.snapshots.some(snapshot=>snapshot.upstreamStale.length)))} onClick={()=>void confirmGroup()}>{runBusy?<Loader2 size={14} className="spin"/>:<Sparkles size={14}/>}确认生成 {groupPreview.params.n} 张</button></div>
  </Modal>}
  {preview && <Modal title="确认本次运行" eyebrow="确认本次运行" onClose={() => !runBusy && setPreview(undefined)} className="run-dialog"><div className={`preview-mode-banner ${preview.mode}`}><span>{preview.mode === 'mock' ? <Sparkles size={16}/> : preview.mode === 'manual' ? <Upload size={16}/> : <ShieldCheck size={16}/>}</span><div><strong>{MODE_NAMES[preview.mode]}</strong><p>{preview.mode==='codex'?'确认后创建单图任务，交给当前 Codex 对话使用原生图像工具。会消耗 Codex 账户额度，尺寸仅作为生成意图，实际像素以回填结果为准。':preview.mode === 'mock' ? '使用本地模拟输出，验证工作流，不调用生成 API。' : preview.mode === 'manual' ? '保存请求快照，在原生工具生成后手动回填。' : '确认后向供应商发送以下提示词与必要素材。'}</p></div></div><div className="preview-target"><span>运行对象</span><strong>{graph.nodes.find(n => n.id === preview.nodeId)?.name || '所选节点'}</strong><span className="small-tag">仅此 1 个节点</span></div><label className="field-label">最终提示词</label><div className="preview-prompt">{preview.snapshot.prompt}</div>{snapshotSources(preview.snapshot).length>0 && <div className="group-preview-references">{snapshotSources(preview.snapshot).map((assetId,index)=><figure key={`${assetId}:${index}`}><img src={thumbnailUrl(assetId)} alt={`固定参考图 ${index+1}`}/><figcaption>参考图 {index+1}</figcaption></figure>)}</div>}<div className={`preview-specs ${preview.mode==='codex'?'native-preview-specs':''}`}><div><span>{preview.mode==='codex'?'生成通道':'模型'}</span><strong title={preview.snapshot.model}>{preview.mode==='codex'?'Codex 原生图像工具':preview.snapshot.model}</strong></div><div><span>{preview.mode==='codex'?'期望尺寸':'尺寸'}</span><strong>{preview.snapshot.params.size}</strong></div>{preview.mode!=='codex'&&<div><span>质量</span><strong>{preview.snapshot.params.quality}</strong></div>}<div><span>数量</span><strong>{preview.snapshot.params.n} 张</strong></div></div>{preview.warnings.map((warning, i) => <div className="notice warning" key={i}><AlertCircle size={14}/><span>{warning}</span></div>)}{preview.snapshot.upstreamStale.length > 0 && <label className="checkbox-label stale-confirm"><input type="checkbox" checked={acceptStale} onChange={e => setAcceptStale(e.target.checked)}/><span>我确认继续使用已固定的旧图。上游不会自动更新。</span></label>}{preview.mode==='codex'?<><div className="native-allowance"><span>本次额度来源</span><strong>Codex 账户额度</strong><p>使用账户现有额度与限制，不代表免费；无法在画布中预估剩余额度或折算费用。</p></div><div className={`notice native-host-notice ${hostContext.connected?'':'warning'}`}><Clock3 size={15}/><span>{hostContext.connected?'已连接当前 Codex 对话。确认后交接请求，宿主开始处理后才显示生成中。':'请从 Codex 打开画布以连接当前对话。确认后只保存待处理任务，可复制交接指令继续。'}</span></div></>:<div className="preview-cost"><span>{preview.mode === 'api' ? '本次保守费用上界' : '本次 API 费用'}</span><strong>{preview.mode === 'api' ? `$${preview.estimatedCost.toFixed(3)}` : '$0.00'}<span> USD</span></strong></div>}<div className="modal-actions"><button className="button secondary" disabled={runBusy} onClick={() => setPreview(undefined)}>返回编辑</button><button className="button primary" disabled={runBusy || (preview.snapshot.upstreamStale.length > 0 && !acceptStale)} onClick={confirmRun}>{runBusy ? <Loader2 size={15} className="spin"/> : <Play size={13} fill="currentColor"/>}{preview.mode==='codex'?(hostContext.connected?'确认并交给 Codex':'确认并保存待处理任务'):preview.mode === 'manual' ? '确认并创建手动任务' : '确认运行'}</button></div></Modal>}
  {viewerAsset && <Modal title="图片预览" headerContent={<><div className="image-dialog-meta"><span>{viewerAsset.width} × {viewerAsset.height} px</span><span>{viewerAsset.byteSize >= 1024 * 1024 ? `${(viewerAsset.byteSize / (1024 * 1024)).toFixed(2)} MB` : `${Math.max(1, Math.round(viewerAsset.byteSize / 1024))} KB`}</span></div><button className="icon-button" aria-label="下载图片" title="下载图片" onClick={() => downloadAsset(viewerAsset)}><Download size={18}/></button></>} onClose={() => setViewer(undefined)} className="image-dialog"><ImageViewerSurface key={viewerAsset.id} src={assetUrl(viewerAsset)} alt={viewerAsset.name || '图片预览'} width={viewerAsset.width} height={viewerAsset.height}/></Modal>}
  {compareOpen && compareAssets.length === 2 && <Modal title="版本对比" eyebrow="版本对比" onClose={() => setCompareOpen(false)} className="compare-dialog"><div className="segmented"><button className={compareMode === 'side' ? 'active' : ''} onClick={() => setCompareMode('side')}>并排对比</button><button className={compareMode === 'slide' ? 'active' : ''} onClick={() => setCompareMode('slide')}>滑动对比</button></div>{compareMode === 'side' ? <div className="comparison-side">{compareAssets.map((asset, i) => <div key={asset.id}><span>{i === 0 ? 'A' : 'B'} · {asset.source === 'codex' ? 'Codex 原生' : asset.source === 'mock' ? '模拟图像' : asset.name}</span><img src={assetUrl(asset)} alt={`对比图 ${i ? 'B' : 'A'}`}/><button className="button secondary compact" onClick={() => { chooseAsset(asset); setCompareOpen(false); }}>采用这个版本</button></div>)}</div> : <div className="comparison-slide"><div className="slider-images"><img src={assetUrl(compareAssets[1])} alt="对比图 B"/><img className="slider-overlay" style={{ clipPath: `inset(0 ${100 - slider}% 0 0)` }} src={assetUrl(compareAssets[0])} alt="对比图 A"/><span className="slider-divider" style={{ left: `${slider}%` }}/><span className="slider-label a">A</span><span className="slider-label b">B</span></div><label className="field-label">拖动查看差异<input type="range" min="0" max="100" aria-label="对比滑块" value={slider} onChange={e => setSlider(Number(e.target.value))}/></label></div>}</Modal>}
  {deleteTarget && <Modal title={deleteTarget.nodeIds.length===1?'删除这张图片？':`删除 ${deleteTarget.nodeIds.length} 张图片？`} onClose={() => !deleteBusy&&setDeleteTarget(undefined)} className="small-dialog delete-dialog"><form onSubmit={event=>{event.preventDefault();void deleteNodes();}}><p className="dialog-copy">移到回收站，可随时恢复。</p><div className="modal-actions"><button type="button" className="button secondary" disabled={deleteBusy} onClick={()=>setDeleteTarget(undefined)}>取消</button><button type="submit" data-autofocus className="button danger" disabled={deleteBusy}>{deleteBusy?<Loader2 size={14} className="spin"/>:<Trash2 size={14}/>}删除<kbd>↵</kbd></button></div></form></Modal>}

  {attachRun && <Modal title="回填生成结果" eyebrow="手动任务" onClose={() => !runBusy && setAttachRun(undefined)} className="run-dialog"><div className="notice"><ShieldCheck size={16}/><span>结果将标记为“用户确认”。工具无法验证外部生成时使用了相同的提示词。</span></div><div className="manual-actions"><button className="button secondary" onClick={() => exportManual(attachRun)}><Download size={14}/>导出提示词</button>{attachRun.inputSnapshot.sourceAssetVersionId && <button className="button secondary" onClick={() => { const asset = assets.find(a => a.id === attachRun.inputSnapshot.sourceAssetVersionId); if (asset)
        downloadAsset(asset); }}><FileImage size={14}/>下载主图</button>}</div><label className="field-label">任务提示词</label><div className="preview-prompt">{attachRun.inputSnapshot.prompt}</div><button className="upload-zone manual-upload" disabled={uploading} onClick={() => { uploadTarget.current = undefined; fileInput.current?.click(); }}><Upload size={23}/><strong>选择外部生成的结果</strong><span>PNG、JPEG、WebP · 最多 {attachRun.inputSnapshot.params.n} 张，可部分回填</span></button>{attachAssets.length > 0 && <div className="attach-thumbnails">{attachAssets.map(aid => <div key={aid}><img src={thumbnailUrl(aid)} alt="待回填图像"/><button aria-label="移除回填图片" onClick={() => setAttachAssets(a => a.filter(x => x !== aid))}><X size={12}/></button></div>)}</div>}<label className="field-label" htmlFor="source-note">来源说明</label><textarea id="source-note" className="note-editor" placeholder="例如：在 ChatGPT 中生成，使用本次提示词和主图" value={sourceNote} onChange={e => setSourceNote(e.target.value)}/><label className="checkbox-label"><input type="checkbox" checked={attachConfirm} onChange={e => setAttachConfirm(e.target.checked)}/><span>我确认这些图片对应此任务，并愿意保留来源说明。</span></label><div className="modal-actions"><button className="text-button cancel-manual" disabled={runBusy} onClick={async()=>{await cancelRun(attachRun);setAttachRun(undefined);}}>取消此任务</button><button className="button secondary" onClick={() => setAttachRun(undefined)}>稍后回填</button><button className="button primary" disabled={!attachConfirm || !attachAssets.length || !sourceNote.trim() || runBusy} onClick={attachResults}>{runBusy ? <Loader2 size={14} className="spin"/> : <Check size={14}/>}关联到此任务</button></div></Modal>}


  {contextMenu&&<CanvasContextMenu {...contextMenu} actions={contextActions} onClose={()=>setContextMenu(undefined)}/>}
  {helpOpen && <Modal title="使用指南" eyebrow="画布使用指南" onClose={() => setHelpOpen(false)} className="help-dialog"><div className="help-flow"><span><ImagePlus size={19}/>选择图片</span><ArrowRight size={16}/><span><SlidersHorizontal size={19}/>侧栏填写</span><ArrowRight size={16}/><span><Play size={19}/>预览确认</span></div><p className="dialog-copy">从顶栏切换或新建画布。点击左侧“新建”开始创作，或导入图片直接编辑。每张图片都有自己的提示词和尺寸设置，选中后在右侧编辑，再预览确认。在左侧历史查找生成结果或打开对应的 Codex 会话。</p><div className="help-shortcuts">{[['空格 + 拖动', '平移画布'], ['拖动空白处', '框选节点'], ['滚轮', '缩放画布'], ['⌘ C / ⌘ V', '复制 / 粘贴节点'], ['⌘ Z / ⇧ ⌘ Z', '撤销 / 重做画布操作'], ['Delete', '删除选中节点或连线'], ['图片上的放大按钮', '查看原始大图'], ['Enter', '预览选中的图片任务']].map(([key, label]) => <div key={key}><span>{label}</span><kbd>{key}</kbd></div>)}</div><div className="notice"><History size={15}/><span>修改上游只会将结果标记为过期；画布不会自动生成。新结果需要手动采用，撤销画布也不会撤回已经提交的任务。</span></div><div className="modal-actions"><button className="button primary" onClick={() => setHelpOpen(false)}>开始创作<ArrowRight size={14}/></button></div></Modal>}
 </div>;
}
function Modal({ title, eyebrow, headerContent, onClose, className = '', children, dismissible = true }: {
    title: string;
    eyebrow?: string;
    headerContent?: React.ReactNode;
    onClose: () => void;
    className?: string;
    dismissible?: boolean;
    children: React.ReactNode;
}) {
    const ref = useRef<HTMLDivElement>(null);
    const closeRef = useRef(onClose);
    closeRef.current = onClose;
    useEffect(() => { const previous = document.activeElement as HTMLElement | null; const el = ref.current; (el?.querySelector<HTMLElement>('[data-autofocus]') || el)?.focus(); const handler = (e: KeyboardEvent) => { if (e.key === 'Escape')
        closeRef.current(); if (e.key === 'Tab' && el) {
        const items = Array.from(el.querySelectorAll<HTMLElement>('button:not(:disabled),input:not(:disabled),textarea:not(:disabled),select:not(:disabled),[tabindex="0"]'));
        const first = items[0], last = items[items.length - 1];
        if (e.shiftKey && (document.activeElement === first || document.activeElement === el)) {
            e.preventDefault();
            last?.focus();
        }
        else if (!e.shiftKey && (document.activeElement === last || document.activeElement === el)) {
            e.preventDefault();
            first?.focus();
        }
    } }; window.addEventListener('keydown', handler); return () => { window.removeEventListener('keydown', handler); previous?.focus(); }; }, []);
    return <div className="modal-backdrop" onMouseDown={e => { if (e.target === e.currentTarget)
        onClose(); }}><div ref={ref} className={`modal ${className}`} role="dialog" aria-modal="true" aria-label={title} tabIndex={-1}><div className="modal-header">{headerContent ?? <div>{eyebrow && <span className="small-eyebrow">{eyebrow}</span>}<h2>{title}</h2></div>}{dismissible && <button className="icon-button" aria-label="关闭弹窗" onClick={onClose}><X size={19}/></button>}</div><div className="modal-content">{children}</div></div></div>;
}

function formatMergeValue(value: unknown): string {
    if (value === undefined || value === null) return '已删除 / 未设置';
    return typeof value === 'string' ? value || '（空白）' : JSON.stringify(value, null, 2);
}
