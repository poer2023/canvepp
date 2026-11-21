import type { ImageSize } from '@/domain/types';
import { formatImageSize, validateImageDimensions } from '@/domain/image-size';

/** Keep incomplete editor text outside the persisted graph. */
export function resolveImageSizeDraft(width: string, height: string): { size?: ImageSize; error?: string } {
    const read = (value: string) => /^[0-9]+$/.test(value) ? Number(value) : undefined;
    const parsedWidth = read(width), parsedHeight = read(height);
    const errors = validateImageDimensions(parsedWidth, parsedHeight);
    return errors.length ? { error: errors.join('；') } : { size: formatImageSize(parsedWidth!, parsedHeight!) };
}

/** Preserve exactly what was typed; only the linked axis is rounded to a pixel. */
export function linkImageDimensions(draft: { width: string; height: string }, field: 'width' | 'height', value: string, ratio: string) {
    const next = { ...draft, [field]: value };
    if (ratio === 'free' || !/^[0-9]+$/.test(value)) return next;
    const [width, height] = ratio.split(':').map(Number);
    if (!(width > 0 && height > 0) || !Number.isSafeInteger(Number(value))) return next;
    const other = field === 'width' ? 'height' : 'width';
    next[other] = String(Math.round(Number(value) * (field === 'width' ? height / width : width / height)));
    return next;
}
