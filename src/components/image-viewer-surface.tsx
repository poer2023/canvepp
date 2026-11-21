'use client';
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react';
import styles from './image-viewer-surface.module.css';

type Props = { src: string; alt: string; width: number; height: number };
type Zoom = 'fit' | number;

/** Percentages refer to original image pixels, not the fitted preview size. */
export function ImageViewerSurface({ src, alt, width, height }: Props) {
    const viewportRef = useRef<HTMLDivElement>(null);
    const [bounds, setBounds] = useState({ width: 0, height: 0 });
    const [view, setView] = useState<{ src: string; zoom: Zoom }>({ src, zoom: 'fit' });
    const zoom = view.src === src ? view.zoom : 'fit';
    const imageWidth = Math.max(1, width), imageHeight = Math.max(1, height);
    const fitScale = bounds.width && bounds.height ? Math.min(1, Math.max(1, bounds.width - 32) / imageWidth, Math.max(1, bounds.height - 32) / imageHeight) : 1;
    const scale = zoom === 'fit' ? fitScale : zoom;
    const displayedWidth = imageWidth * scale, displayedHeight = imageHeight * scale;
    const stageWidth = Math.max(bounds.width, displayedWidth + 32);
    const stageHeight = Math.max(bounds.height, displayedHeight + 32);
    const centerRef = useRef<{ x: number; y: number; viewportX: number; viewportY: number } | null>(null);
    const dragRef = useRef<{ pointerId: number; x: number; y: number; left: number; top: number } | null>(null);

    useEffect(() => {
        const viewport = viewportRef.current;
        if (!viewport) return;
        const measure = () => {
            const next = { width: viewport.clientWidth, height: viewport.clientHeight };
            setBounds(previous => previous.width === next.width && previous.height === next.height ? previous : next);
        };
        measure();
        const observer = new ResizeObserver(measure);
        observer.observe(viewport);
        return () => observer.disconnect();
    }, []);
    useEffect(() => {
        centerRef.current = null;
        setView({ src, zoom: 'fit' });
        viewportRef.current?.scrollTo({ left: 0, top: 0 });
    }, [src]);
    useLayoutEffect(() => {
        const viewport = viewportRef.current, center = centerRef.current;
        if (!viewport || !center) return;
        centerRef.current = null;
        viewport.scrollLeft = center.x * scale + (stageWidth - displayedWidth) / 2 - center.viewportX;
        viewport.scrollTop = center.y * scale + (stageHeight - displayedHeight) / 2 - center.viewportY;
    }, [scale, stageWidth, stageHeight, displayedWidth, displayedHeight]);

    const setZoom = useCallback((next: Zoom, point?: { x: number; y: number }) => {
        const viewport = viewportRef.current;
        if (viewport) {
            const viewportX = point?.x ?? viewport.clientWidth / 2;
            const viewportY = point?.y ?? viewport.clientHeight / 2;
            centerRef.current = {
                x: (viewport.scrollLeft + viewportX - (stageWidth - displayedWidth) / 2) / scale,
                y: (viewport.scrollTop + viewportY - (stageHeight - displayedHeight) / 2) / scale,
                viewportX, viewportY,
            };
        }
        setView({ src, zoom: next });
    }, [src, scale, stageWidth, stageHeight, displayedWidth, displayedHeight]);
    useEffect(() => {
        const viewport = viewportRef.current;
        if (!viewport) return;
        const wheel = (event: WheelEvent) => {
            event.preventDefault();
            const delta = event.deltaY * (event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? viewport.clientHeight : 1);
            if (!delta) return;
            const bounds = viewport.getBoundingClientRect();
            setZoom(Math.min(4, Math.max(Math.min(.1, fitScale), scale * Math.exp(-delta * .002))), { x: event.clientX - bounds.left, y: event.clientY - bounds.top });
        };
        viewport.addEventListener('wheel', wheel, { passive: false });
        return () => viewport.removeEventListener('wheel', wheel);
    }, [setZoom, scale, fitScale]);
    const stepZoom = (direction: number) => setZoom(Math.min(4, Math.max(.1, Math.round((scale + direction * .25) * 100) / 100)));
    return <section className={styles.viewer} aria-label="原图缩放查看器">
        <div ref={viewportRef} className={styles.viewport} tabIndex={0} data-zoom={scale} data-zoomed={scale > fitScale} aria-label="图像预览。滚轮缩放，拖动查看细节；加减键缩放，0 恢复适应，1 原始像素，双击切换适应与原始像素。"
            onDoubleClick={() => setZoom(zoom === 'fit' ? 1 : 'fit')}
            onKeyDown={event => {
                if (event.ctrlKey || event.metaKey || event.altKey) return;
                if (!['+', '=', '-', '0', '1'].includes(event.key)) return;
                event.preventDefault(); event.stopPropagation();
                if (event.key === '0') setZoom('fit');
                else if (event.key === '1') setZoom(1);
                else stepZoom(event.key === '-' ? -1 : 1);
            }}
            onPointerDown={event => {
                if (event.button !== 0 || event.pointerType === 'touch') return;
                event.currentTarget.focus({ preventScroll: true });
                if (scale <= fitScale) return;
                event.preventDefault();
                dragRef.current = { pointerId: event.pointerId, x: event.clientX, y: event.clientY, left: event.currentTarget.scrollLeft, top: event.currentTarget.scrollTop };
                event.currentTarget.setPointerCapture(event.pointerId);
            }}
            onPointerMove={event => {
                const drag = dragRef.current;
                if (!drag || drag.pointerId !== event.pointerId) return;
                event.currentTarget.scrollLeft = drag.left + drag.x - event.clientX;
                event.currentTarget.scrollTop = drag.top + drag.y - event.clientY;
            }}
            onPointerUp={event => { dragRef.current = null; if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId); }}
            onPointerCancel={() => { dragRef.current = null; }} onLostPointerCapture={() => { dragRef.current = null; }}>
            <div className={styles.stage} style={{ width: stageWidth || '100%', height: stageHeight || '100%' }}>
                <img src={src} alt={alt} draggable={false} width={imageWidth} height={imageHeight} className={styles.image} style={{ width: displayedWidth, height: displayedHeight }}/>
            </div>
        </div>
    </section>;
}
