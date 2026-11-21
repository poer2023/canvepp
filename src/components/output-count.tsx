'use client';
import { Images } from 'lucide-react';

export function OutputCount({ value, disabled, onChange }: { value: number; disabled?: boolean; onChange: (value: number) => void }) {
    return <section className="output-count-control">
        <div className="field-label"><span className="count-label"><Images size={14}/>生成张数</span><span>{value} 张</span></div>
        <div className="output-count-options" role="group" aria-label="生成张数">{[1,2,3,4].map(count => <button type="button" key={count} disabled={disabled} aria-label={`生成 ${count} 张`} aria-pressed={value === count} onClick={() => onChange(count)}><span className="count-stack" aria-hidden="true">{Array.from({length:count},(_,index)=><i key={index} style={{transform:`translate(${index*3}px,${-index*2}px)`}}/>)}</span><span>{count}</span></button>)}</div>
        {value > 1 && <div className="output-count-preview" aria-label={`输出 ${value} 张独立图片卡片`}><span className="output-origin"><Images size={13}/></span><span className="output-line"/><span className="output-results">{Array.from({length:value},(_,index)=><span key={index}>{index+1}</span>)}</span><small>独立图片</small></div>}
    </section>;
}

