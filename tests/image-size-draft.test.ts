import { describe, expect, it } from 'vitest';
import { resolveImageSizeDraft, linkImageDimensions } from '../src/lib/image-size-draft';

describe('image size editor drafts', () => {
    it('links fixed proportions both ways while preserving the edited axis, and leaves free pixels independent', () => {
        const draft = { width: '1920', height: '1080' };
        expect(linkImageDimensions(draft, 'width', '1280', '16:9')).toEqual({ width: '1280', height: '720' });
        expect(linkImageDimensions(draft, 'height', '900', '16:9')).toEqual({ width: '1600', height: '900' });
        expect(linkImageDimensions(draft, 'width', '1000', '16:9')).toEqual({ width: '1000', height: '563' });
        expect(linkImageDimensions(draft, 'width', '1000', 'free')).toEqual({ width: '1000', height: '1080' });
        expect(linkImageDimensions(draft, 'width', '', '16:9')).toEqual({ width: '', height: '1080' });
        const oversized = linkImageDimensions(draft, 'height', '8192', '16:9');
        expect(resolveImageSizeDraft(oversized.width, oversized.height).size).toBeUndefined();
    });
    it('does not commit incomplete or noninteger pixel text', () => {
        for (const width of ['', '6', '0', '100.5', '1e3', '-100', 'NaN']) {
            const result = resolveImageSizeDraft(width, '1080');
            expect(result.size).toBeUndefined();
            expect(result.error).toBeTruthy();
        }
        expect(resolveImageSizeDraft('1920', '').size).toBeUndefined();
    });
    it('commits exact valid pixels and rejects per-side and total limits', () => {
        expect(resolveImageSizeDraft('1919', '1080')).toEqual({ size: '1919x1080' });
        expect(resolveImageSizeDraft('64', '8192')).toEqual({ size: '64x8192' });
        expect(resolveImageSizeDraft('8193', '1080').error).toContain('8192');
        expect(resolveImageSizeDraft('8192', '8192').error).toContain('32 MP');
    });
});
