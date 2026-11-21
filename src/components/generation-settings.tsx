'use client';
import { useCallback, useEffect, useState } from 'react';
import { ArrowLeft, ArrowRight, Images, Layers2, Loader2, Plus, Sparkles, X } from 'lucide-react';
import type { AssetVersion, CanvasNode, ImageParams, RunMode } from '@/domain/types';
import { DEFAULT_PARAMS } from '@/domain/graph';
import { ImageControls } from './image-controls';


interface MultiProps {
    projectId: string;
    nodes: CanvasNode[];
    assets: AssetVersion[];
    mode: RunMode;
    busy: boolean;
    disabledReason?: string;
    onGenerate: (nodeIds: string[], prompt: string, params: ImageParams) => void;
    onView: (assetId: string) => void;
}
export function MultiReferenceSettings({projectId,nodes,assets,mode,busy,disabledReason,onGenerate,onView}:MultiProps) {
    const key=`canvepp:reference-draft:${projectId}:${nodes.map(n=>n.id).sort().join(',')}`;
    const [draft,setDraft]=useState(()=>{
        try{const saved=JSON.parse(sessionStorage.getItem(key)||'null');if(saved&&typeof saved.prompt==='string'&&Array.isArray(saved.order)&&saved.params)return saved as {prompt:string;params:ImageParams;order:string[]};}catch{}
        return {prompt:'',params:{...DEFAULT_PARAMS},order:nodes.map(node=>node.id)};
    });
    const [sizeError,setSizeError]=useState<string>();
    const [storageError,setStorageError]=useState(false);
    const validity=useCallback((_id:string,error?:string)=>setSizeError(error),[]);
    useEffect(()=>{try{sessionStorage.setItem(key,JSON.stringify(draft));setStorageError(false);}catch{setStorageError(true);}},[key,draft]);
    const references=draft.order.map(id=>nodes.find(node=>node.id===id)).filter(Boolean) as CanvasNode[];
    const pictures=references.map(node=>assets.find(asset=>asset.id===node.selectedAssetVersionId));
    const missing=pictures.filter(asset=>!asset).length;
    const reason=disabledReason||sizeError||(references.length<2?'至少选择 2 张参考图':references.length>8?'最多选择 8 张参考图':missing?`有 ${missing} 张空白卡片，请先移出或采用图片`:!draft.prompt.trim()?'请描述这些参考图如何组合':mode==='api'?'多图参考请切换到 Codex 原生模式':undefined);
    const reorder=(index:number,offset:number)=>{const order=[...draft.order],next=index+offset;if(next<0||next>=order.length)return;[order[index],order[next]]=[order[next],order[index]];setDraft({...draft,order});};
    return <>
        <div className="inspector-scroll multi-reference-scroll">
            <section className="multi-reference-intro"><div className="reference-stack-symbol"><Layers2 size={22}/><b>{references.length}</b></div><div><strong>多图共同参考</strong><p>{references.length} 张参考图 → {draft.params.n} 张新图片</p></div></section>
            <section className="property-section reference-section">
                <div className="field-label">参考图片<span>{references.length} 张</span></div>
                <div className="reference-grid">{references.map((node,index)=><article key={node.id} className={`reference-tile ${pictures[index]?'':'is-missing'}`}>
                    <button className="reference-preview" aria-label={`查看参考图 ${index+1}`} disabled={!pictures[index]} onClick={()=>onView(pictures[index]!.id)}>{pictures[index]?<img src={`/api/assets/${pictures[index]!.id}?thumbnail=1`} alt={node.name} draggable={false}/>:<span><Images size={20}/>空白图片</span>}</button>
                    <span className="reference-number">{index+1}</span><button className="reference-remove" aria-label={`移出参考图 ${index+1}`} title="仅移出本次参考，保留原图" onClick={()=>setDraft({...draft,order:draft.order.filter(id=>id!==node.id)})}><X size={12}/></button>
                    <div className="reference-caption"><span title={node.name}>{node.name}</span><button disabled={index===0} aria-label={`前移参考图 ${index+1}`} onClick={()=>reorder(index,-1)}><ArrowLeft size={11}/></button><button disabled={index===references.length-1} aria-label={`后移参考图 ${index+1}`} onClick={()=>reorder(index,1)}><ArrowRight size={11}/></button></div>
                </article>)}</div>
                {references.length<nodes.length&&<button className="reference-restore" onClick={()=>setDraft({...draft,order:nodes.map(node=>node.id)})}><Plus size={12}/>恢复全部所选图片</button>}
                {(missing>0||references.length<2||references.length>8)&&<p className="reference-validation" role="alert">{missing?`有 ${missing} 张空白图片，请移出或先采用图片。`:references.length<2?'至少需要 2 张参考图。':'最多同时参考 8 张图片。'}</p>}
            </section>
            <section className="property-section multi-prompt-section"><label className="field-label" htmlFor="multi-image-prompt">组合提示词<span>{draft.prompt.length} 字</span></label><textarea id="multi-image-prompt" className="prompt-editor image-prompt-editor" aria-label="多图参考提示词" maxLength={32000} placeholder="例如：保留图 1 的人物，使用图 2 的场景，参考图 3 的色调…" value={draft.prompt} onChange={event=>setDraft({...draft,prompt:event.target.value})}/>{storageError&&<p role="alert" className="reference-validation">草稿暂时无法保存在浏览器，请保持此面板打开。</p>}</section>
            <section className="property-section image-size-section"><ImageControls nodeId="multi-reference" params={draft.params} mode={mode} connected={false} onChange={params=>setDraft({...draft,params})} onValidity={validity}/></section>
        </div>
        <div className="inspector-run-footer multi-run-footer"><div className="multi-output-summary"><Layers2 size={13}/><span>{references.length} 张共同参考</span><ArrowRight size={13}/><span>{draft.params.n} 张新图片</span></div><button className="button primary full run-primary" title={reason||'共同参考所选图片生成'} disabled={busy||!!reason} onClick={()=>onGenerate(draft.order,draft.prompt,draft.params)}>{busy?<Loader2 size={15} className="spin"/>:<Sparkles size={15}/>}<span>参考 {references.length} 张，生成 {draft.params.n} 张</span></button></div>
    </>;
}
