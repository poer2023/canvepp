'use client';
import { ChevronDown, Clock3, ExternalLink, ImageIcon, Search } from 'lucide-react';
import { useMemo, useState, type ReactNode } from 'react';
import type { AssetVersion, CanvasNode, Run } from '@/domain/types';
import { generationGroup, snapshotSources } from '@/domain/generation-group';

function executionLink(run: Run) {
    const trace = (run.usage?.canvasExecution ?? run.usage?.hostDispatch) as { threadId?: string; turnId?: string } | undefined;
    if (!trace?.threadId || !/^[a-f0-9]{8}(?:-[a-f0-9]{4}){3}-[a-f0-9]{12}$/i.test(trace.threadId)) return;
    return { url: 'codex://threads/' + trace.threadId, turn: trace.turnId };
}
function dayLabel(value: string) {
    const date = new Date(value), today = new Date(), yesterday = new Date();
    yesterday.setDate(today.getDate() - 1);
    if (date.toDateString() === today.toDateString()) return '今天';
    if (date.toDateString() === yesterday.toDateString()) return '昨天';
    return date.toLocaleDateString('zh-CN', {year:'numeric',month:'numeric',day:'numeric'});
}
interface Props {
    runs: Run[]; nodes: CanvasNode[]; assets: AssetVersion[]; selectedId?: string;
    onSelect: (run?: Run) => void; onLocate: (runs: Run[]) => void;
    onView: (id: string) => void; status: (run: Run) => string; children: ReactNode;
}
export function RunHistory({runs,nodes,assets,selectedId,onSelect,onLocate,onView,status,children}:Props) {
    const [query,setQuery] = useState('');
    const [days,setDays] = useState('all');
    const byId = useMemo(() => new Map(assets.map(asset => [asset.id,asset])),[assets]);
    const groups = useMemo(() => {
        const grouped = new Map<string,Run[]>();
        for (const run of runs) {
            const key = generationGroup(run)?.id ?? run.id;
            const group = grouped.get(key) ?? [];
            group.push(run); grouped.set(key,group);
        }
        return [...grouped.values()].map(group => group.sort((a,b) => (generationGroup(a)?.index ?? 0) - (generationGroup(b)?.index ?? 0)));
    },[runs]);
    const term = query.trim().toLocaleLowerCase();
    const visible = groups.filter(group => group.some(run => {
        if (days !== 'all' && Date.parse(run.createdAt) < Date.now()-Number(days)*86400000) return false;
        return !term || [run.inputSnapshot.prompt, status(run), dayLabel(run.createdAt), run.createdAt.slice(0,10),
            nodes.find(node=>node.id===run.nodeId)?.name, ...[...run.assetVersionIds,...snapshotSources(run.inputSnapshot)].map(id=>byId.get(id)?.name)]
            .some(text=>text?.toLocaleLowerCase().includes(term));
    }));
    return <>
        <div className="library-tools">
            <div className="library-search"><Search size={14}/><input type="search" aria-label="搜索生成历史" placeholder="搜索图片或提示词" value={query} onChange={e=>setQuery(e.target.value)}/></div>
            <div className="library-filter"><span>{visible.length} 次生成</span><select aria-label="生成日期范围" value={days} onChange={e=>setDays(e.target.value)}><option value="all">所有时间</option><option value="7">最近 7 天</option><option value="30">最近 30 天</option></select></div>
        </div>
        <div className="run-history-scroll" aria-label="生成历史">
            {!visible.length && <div className="library-empty"><Clock3 size={24}/><span>{groups.length?'没有匹配的生成':'还没有生成记录'}</span></div>}
            {visible.map((group,index) => {
                const first=group[0], active=group.find(run=>run.id===selectedId);
                const outputs=group.flatMap(run=>run.assetVersionIds).map(id=>byId.get(id)).filter((a):a is AssetVersion=>!!a);
                const total=group.reduce((n,r)=>n+r.inputSnapshot.params.n,0);
                const label=group.every(r=>r.status===first.status)?status(first):group.some(r=>['running','awaiting_host','queued','submitting'].includes(r.status))?'进行中':group.some(r=>r.status==='unknown')?'待确认':outputs.length?'部分完成':'已结束';
                const trace=active && executionLink(active);
                const date=dayLabel(first.createdAt);
                return <section key={first.id}>
                    {(!index || dayLabel(visible[index-1][0].createdAt)!==date) && <h3 className="run-day">{date}</h3>}
                    <article className={'run-history-card'+(active?' is-open':'')}>
                        <button className="run-history-summary" aria-expanded={!!active} onClick={()=>{if(!active)onLocate(group);onSelect(active?undefined:first);}}>
                            <span className="run-thumbs" data-count={Math.min(outputs.length,4)}>{outputs.length?outputs.slice(0,4).map(asset=><img key={asset.id} src={`/api/assets/${asset.id}?thumbnail=1`} alt="" loading="lazy"/>):<ImageIcon size={17}/>}</span>
                            <span className="run-caption"><strong>{first.inputSnapshot.prompt || '图片生成'}</strong><small>{total} 张 · {label} · {new Date(first.createdAt).toLocaleTimeString('zh-CN',{hour:'2-digit',minute:'2-digit'})}</small></span>
                            <ChevronDown size={13}/>
                        </button>
                        {active && <div className="history-run-details">
                            {group.length>1 && <div className="run-members" aria-label="批次结果">{group.map((r,i)=><button key={r.id} aria-pressed={r.id===active.id} onClick={()=>{onSelect(r);onLocate([r]);}} title={status(r)}>{i+1} · {status(r)}</button>)}</div>}
                            {!!snapshotSources(active.inputSnapshot).length && <div className="run-reference-strip"><small>参考图</small>{snapshotSources(active.inputSnapshot).map((id,i)=><button key={i} aria-label={'查看参考图 '+(i+1)} onClick={()=>onView(id)}><img src={`/api/assets/${id}?thumbnail=1`} alt=""/></button>)}</div>}
                            {children}
                            {trace && <a className="run-chat-link" href={trace.url} title={trace.turn?'执行轮次 '+trace.turn:undefined}>查看 Codex 会话<ExternalLink size={12}/></a>}
                        </div>}
                    </article>
                </section>;
            })}
        </div>
    </>;
}
