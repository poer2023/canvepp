'use client';
import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { Check, ChevronDown, ImageIcon, Loader2, LocateFixed, RefreshCw, Search, X } from 'lucide-react';
import { filterLibrary, type LibraryImage, type LibraryPage } from '@/domain/image-library';
import type { AssetVersion } from '@/domain/types';

interface Props {
    view:'images'|'runs';
    onViewChange:(view:'images'|'runs')=>void;
    activity:ReactNode;
    items: LibraryImage[];
    projectId: string;
    activeAssetId?: string;
    activeName?: string;
    versionCount: number;
    versionsOpen: boolean;
    onVersionsOpen: (value: boolean) => void;
    onClose: () => void;
    onView: (asset: AssetVersion) => void;
    onLocate: (item: LibraryImage) => Promise<void>;
    fetchPage: (query: string) => Promise<LibraryPage>;
    children: ReactNode;
}
export function ImageLibrary({ view,onViewChange,activity,items, projectId, activeAssetId, activeName, versionCount, versionsOpen, onVersionsOpen, onClose, onView, onLocate, fetchPage, children }: Props) {
    const [scope, setScope] = useState<'canvas' | 'all'>('canvas');
    const [query, setQuery] = useState('');
    const [days, setDays] = useState('all');
    const [page, setPage] = useState<LibraryPage>({ items: [], total: 0 });
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');
    const [refresh, setRefresh] = useState(0);
    const request = useRef(0);
    const since = useMemo(() => days === 'all' ? '' : new Date(Date.now() - Number(days) * 86400000).toISOString(), [days, refresh]);
    const local = useMemo(() => filterLibrary(items, query, since), [items, query, since]);
    const contentStamp = items.map(item => `${item.asset.id}:${item.used}`).join(',');
    const parameters = new URLSearchParams({ q: query.trim(), since, limit: '60' }).toString();
    useEffect(() => {
        const current = ++request.current;
        if (scope !== 'all') { setBusy(false); setError(''); return; }
        setBusy(true); setError(''); setPage({ items: [], total: 0 });
        const timer = setTimeout(() => {
            fetchPage(parameters).then(value => { if (current === request.current) setPage(value); })
                .catch(error => { if (current === request.current) setError(error.message); })
                .finally(() => { if (current === request.current) setBusy(false); });
        }, 180);
        return () => { clearTimeout(timer); request.current++; };
    }, [scope, parameters, projectId, contentStamp, refresh, fetchPage]);
    const loadMore = async () => {
        const current = request.current;
        setBusy(true); setError('');
        try {
            const next = await fetchPage(`${parameters}&offset=${page.items.length}`);
            if (current === request.current) setPage(old => ({ total: next.total, items: [...old.items, ...next.items.filter(item => !old.items.some(prior => prior.asset.id === item.asset.id))] }));
        } catch (error) { if (current === request.current) setError((error as Error).message); }
        finally { if (current === request.current) setBusy(false); }
    };
    const visible = scope === 'canvas' ? local : page.items;
    const total = scope === 'canvas' ? local.length : page.total;
    return <>
        <div className="drawer-heading library-heading"><strong>历史</strong><div className="library-heading-actions">{view==='images' && <button className="icon-button" aria-label="刷新图片库" title="刷新图片库" onClick={() => setRefresh(value => value + 1)}><RefreshCw size={14}/></button>}<button className="icon-button" aria-label="关闭历史" onClick={onClose}><X size={16}/></button></div></div>
        <div className="history-view-tabs" role="tablist" aria-label="历史类型"><button role="tab" aria-selected={view==='images'} onClick={()=>onViewChange('images')}>图片</button><button role="tab" aria-selected={view==='runs'} onClick={()=>onViewChange('runs')}>生成记录</button></div>
        {view==='runs'?activity:<>
        <div className="library-tools">
            <div className="library-tabs" role="tablist" aria-label="图片范围"><button role="tab" aria-selected={scope === 'canvas'} onClick={() => setScope('canvas')}>本画布</button><button role="tab" aria-selected={scope === 'all'} title="查看所有画布保存的图片" onClick={() => setScope('all')}>全部画布</button></div>
            <div className="library-search"><Search size={14}/><input type="search" aria-label="搜索图片或提示词" maxLength={300} placeholder="搜索图片或提示词" value={query} onChange={event => setQuery(event.target.value)}/></div>
            <div className="library-filter"><span>{busy ? '正在查找…' : `${total} 张图片`}</span><select aria-label="图片日期范围" value={days} onChange={event => setDays(event.target.value)}><option value="all">所有时间</option><option value="7">最近 7 天</option><option value="30">最近 30 天</option></select></div>
        </div>
        <div className="library-scroll" aria-label="图片缩略图列表" aria-busy={busy}>
            {error && <div className="library-error" role="alert">{error}<button onClick={() => setRefresh(value => value + 1)}>重试</button></div>}
            {!visible.length && !busy && !error && <div className="library-empty"><ImageIcon size={26} strokeWidth={1.2}/><span>{query || days !== 'all' ? '没有匹配的图片' : '图片将在这里保留'}</span></div>}
            <div className="library-grid">{visible.map(item => <article className={`library-item ${item.asset.id === activeAssetId ? 'is-selected' : ''}`} key={item.asset.id}>
                <div className="library-thumbnail">
                    <button className="library-image-button" title={item.prompt || item.asset.name} aria-label={`预览图片 ${item.asset.name}`} onClick={() => onView(item.asset)}><img src={`/api/assets/${item.asset.id}?thumbnail=1`} alt={item.asset.name} loading="lazy" decoding="async" draggable={false}/></button>
                    {item.used && <span className="library-used" title="画布中正在使用"><Check size={11}/></span>}
                    {item.asset.source === 'mock' && <span className="library-mock">模拟</span>}
                    <button className="library-locate" title={item.nodeId ? '定位到图片' : '打开所属画布，素材仍保留在图片库'} aria-label={`定位图片 ${item.asset.name}`} onClick={() => void onLocate(item)}><LocateFixed size={14}/></button>
                </div>
                <button className="library-image-name" title={item.asset.name} onClick={() => void onLocate(item)}>{item.asset.name}</button>
                <span className="library-caption" title={scope === 'all' ? item.projectName : undefined}>{scope === 'all' ? item.projectName : new Date(item.asset.createdAt).toLocaleDateString('zh-CN', { month: 'short', day: 'numeric' })}</span>
            </article>)}</div>
            {scope === 'all' && page.items.length < page.total && <button className="library-load-more" disabled={busy} onClick={() => void loadMore()}>{busy ? <Loader2 size={14} className="spin"/> : '加载更多'}</button>}
        </div>
        <div className={`library-versions ${versionsOpen ? 'is-open' : ''}`}>
            <button className="library-versions-toggle" aria-expanded={versionsOpen} onClick={() => onVersionsOpen(!versionsOpen)}><span>{activeName || '画布图片'} · 版本</span><small>{versionCount}</small><ChevronDown size={14}/></button>
            {versionsOpen && <div className="library-version-scroll">{children}</div>}
        </div>
        </>}
    </>;
}
