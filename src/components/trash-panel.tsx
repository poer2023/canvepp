'use client';
import { ImageIcon, Loader2, RotateCcw, Trash2, X } from 'lucide-react';
import type { TrashedCard } from '@/domain/types';
export function TrashPanel({items,busy,error,onClose,onRestore,onView,onRefresh}:{items:TrashedCard[];busy:boolean;error?:string;onClose:()=>void;onRestore:(ids:string[])=>void;onView:(id:string)=>void;onRefresh:()=>void}) {
    return <>
        <div className="drawer-heading"><strong>回收站 <small>{items.length}</small></strong><button className="icon-button" aria-label="关闭回收站" onClick={onClose}><X size={16}/></button></div>
        <div className="trash-toolbar"><span>图片与内容会保留</span><button disabled={busy||!items.length} onClick={()=>onRestore(items.map(item=>item.id))}><RotateCcw size={12}/>全部恢复</button></div>
        <div className="trash-scroll" aria-busy={busy}>{error&&<div className="library-error" role="alert">{error}<button onClick={onRefresh}>重试</button></div>}{busy&&<div className="trash-loading"><Loader2 className="spin" size={18}/></div>}
            {!items.length&&!busy&&!error&&<div className="library-empty"><Trash2 size={27} strokeWidth={1.2}/><span>回收站是空的</span><small>删除的图片可以在这里恢复</small></div>}
            <div className="trash-grid">{items.map(item=><article className="trash-card" key={item.id}><button className="trash-thumbnail" aria-label={`预览已删除图片 ${item.node.name}`} disabled={!item.displayAssetVersionId} onClick={()=>item.displayAssetVersionId&&onView(item.displayAssetVersionId)}>{item.displayAssetVersionId?<img src={`/api/assets/${item.displayAssetVersionId}?thumbnail=1`} alt={item.node.name} draggable={false}/>:<ImageIcon size={24} strokeWidth={1.2}/>}</button><strong title={item.node.name}>{item.node.name}</strong><small>{item.legacy?'历史图片':new Date(item.deletedAt).toLocaleString('zh-CN',{month:'short',day:'numeric',hour:'2-digit',minute:'2-digit'})}</small><button className="trash-restore" disabled={busy} onClick={()=>onRestore([item.id])}><RotateCcw size={12}/>恢复</button></article>)}</div>
        </div>
    </>;
}
