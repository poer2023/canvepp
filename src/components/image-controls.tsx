'use client';
import { useEffect, useRef, useState } from 'react';
import { OutputCount } from './output-count';
import { AlertCircle } from 'lucide-react';
import type { ImageParams, RunMode } from '@/domain/types';
import { IMAGE_SIZE_PRESETS, parseImageSize } from '@/domain/image-size';
import { resolveImageSizeDraft, linkImageDimensions } from '@/lib/image-size-draft';

export const API_IMAGE_SIZES = ['1024x1024', '1536x1024', '1024x1536'] as const;
const apiSupportsSize = (size: string) => (API_IMAGE_SIZES as readonly string[]).includes(size);

type Props = {
    nodeId: string;
    params: ImageParams;
    mode: RunMode;
    connected: boolean;
    onChange: (params: ImageParams) => void;
    onValidity: (nodeId: string, error?: string) => void;
    onSingleImage?: () => void;
};
export function ImageControls({ nodeId, params, mode, connected, onChange, onValidity }: Props) {
    const dimensions = parseImageSize(params.size);
    const inferRatio = (size: string) => { const d = parseImageSize(size); return IMAGE_SIZE_PRESETS.find(p => d && (d.height === Math.round(d.width * p.height / p.width) || d.width === Math.round(d.height * p.width / p.height)))?.ratio ?? 'free'; };
    const [ratio, setRatio] = useState(() => inferRatio(params.size));
    const [draft, setDraft] = useState({ width: dimensions ? String(dimensions.width) : '', height: dimensions ? String(dimensions.height) : '' });
    const lastCommittedSize = useRef(params.size);
    const draftResult = resolveImageSizeDraft(draft.width, draft.height);
    const sizeError = mode === 'api' && !apiSupportsSize(params.size) ? '当前 API 适配器仅支持 1024×1024、1536×1024 和 1024×1536。请选择支持的尺寸。' : draftResult.error;

    useEffect(() => {
        if (lastCommittedSize.current === params.size) return;
        lastCommittedSize.current = params.size;
        const next = parseImageSize(params.size);
        setDraft({ width: next ? String(next.width) : '', height: next ? String(next.height) : '' });
        setRatio(inferRatio(params.size));
    }, [params.size]);
    useEffect(() => { onValidity(nodeId, sizeError); }, [nodeId, sizeError, onValidity]);
    useEffect(() => () => onValidity(nodeId, undefined), [nodeId, onValidity]);

    const selectSize = (size: ImageParams['size'], selectedRatio: string) => {
        const next = parseImageSize(size)!;
        setRatio(selectedRatio);
        setDraft({ width: String(next.width), height: String(next.height) });
        lastCommittedSize.current = size;
        onValidity(nodeId, undefined);
        onChange({ ...params, size });
    };
    const editDimension = (field: 'width' | 'height', value: string) => {
        const next = linkImageDimensions(draft, field, value, ratio);
        setDraft(next);
        const result = resolveImageSizeDraft(next.width, next.height);
        onValidity(nodeId, result.error);
        if (result.size) {
            lastCommittedSize.current = result.size;
            onChange({ ...params, size: result.size });
        }
    };
    return <div className="image-controls">
        <label className="field-label">图片尺寸</label>
        <div className="image-size-options" role="group" aria-label="图片比例">
            {IMAGE_SIZE_PRESETS.map(preset => <button type="button" key={preset.ratio} title={`${preset.width} × ${preset.height} px`} aria-label={`比例 ${preset.ratio}`} disabled={connected || mode === 'api' && !apiSupportsSize(preset.size)} className={ratio === preset.ratio ? 'active' : ''} aria-pressed={ratio === preset.ratio} onClick={() => selectSize(preset.size, preset.ratio)}>
                <span className="size-preset-icon" style={{ width: Math.min(22, 22 * preset.width / preset.height), height: Math.min(22, 22 * preset.height / preset.width) }}/><span>{preset.ratio}</span>
            </button>)}
            <button type="button" className={`custom-size-option ${ratio === 'free' ? 'active' : ''}`} aria-pressed={ratio === 'free'} disabled={connected || mode === 'api'} onClick={() => setRatio('free')}>自由像素</button>
        </div>
        <div className="custom-size-fields">
            {(['width', 'height'] as const).map((field, index) => <div key={field} className="pixel-field">
                <label htmlFor={`image-${field}-${nodeId}`}>{index === 0 ? '宽' : '高'}</label>
                <input id={`image-${field}-${nodeId}`} aria-label={index === 0 ? '图片宽度像素' : '图片高度像素'} type="text" inputMode="numeric" value={draft[field]} disabled={connected || mode === 'api'} aria-invalid={!!sizeError} aria-describedby={sizeError ? `size-error-${nodeId}` : undefined} onChange={event => editDimension(field, event.target.value)}/>
                <span>px</span>
            </div>)}
        </div>
        {sizeError && <p className="size-validation-error" role="alert" id={`size-error-${nodeId}`}><AlertCircle size={13}/><span>{sizeError}</span></p>}
        <OutputCount value={params.n} disabled={connected} onChange={n=>onChange({...params,n})}/>
        {mode === 'api' && <details className="other-image-params"><summary>图像质量</summary><select aria-label="图像质量" className="form-select" disabled={connected} value={params.quality} onChange={event=>onChange({...params,quality:event.target.value as ImageParams['quality']})}><option value="low">标准</option><option value="medium">精细</option><option value="high">高品质</option></select></details>}
    </div>;
}
