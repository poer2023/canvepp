import { describe, expect, it } from 'vitest';
import { formatImageSize, IMAGE_SIZE_LIMITS, IMAGE_SIZE_PRESETS, imageSizeRatio, parseImageSize, validateImageDimensions, validateImageSize } from '../src/domain/image-size';

describe('image dimensions and canonical size strings', () => {
  it('provides the eight requested exact aspect ratios with valid pixels', () => {
    expect(IMAGE_SIZE_PRESETS.map(preset => preset.ratio)).toEqual(['1:1', '4:3', '3:4', '3:2', '2:3', '16:9', '9:16', '21:9']);
    for (const preset of IMAGE_SIZE_PRESETS) {
      const [rw, rh] = preset.ratio.split(':').map(Number);
      expect(preset.width * rh).toBe(preset.height * rw);
      expect(validateImageSize(preset.size)).toEqual([]);
      expect(parseImageSize(preset.size)).toEqual({ width: preset.width, height: preset.height });
      expect(imageSizeRatio(preset.size)).toBe(preset.ratio);
    }
  });

  it.each([[64, 64], [8192, 64], [64, 8192], [8000, 4000], [8192, 3906], [7001, 1234]])('preserves valid %i × %i pixels exactly', (width, height) => {
    const size = formatImageSize(width, height);
    expect(size).toBe(`${width}x${height}`);
    expect(parseImageSize(size)).toEqual({ width, height });
    expect(validateImageDimensions(width, height)).toEqual([]);
  });

  it('enforces the inclusive 32 MP boundary independently from side limits', () => {
    expect(IMAGE_SIZE_LIMITS).toEqual({ minSide: 64, maxSide: 8192, maxPixels: 32_000_000 });
    expect(validateImageSize('8000x4000')).toEqual([]);
    expect(validateImageSize('8000x4001').join()).toContain('32 MP');
    expect(validateImageSize('8192x3907').join()).toContain('32 MP');
    expect(parseImageSize('8192x8192')).toBeUndefined();
    expect(() => formatImageSize(8192, 8192)).toThrow('32 MP');
  });

  it.each([undefined, null, 1024, NaN, {}, [], '', '64', '0x64', '-64x64', '63x64', '64x63', '8193x64', '64x8193', '64.5x64', '1e3x64', 'NaNx64', 'Infinityx64', '064x64', '64x0064', '64X64', '64×64', '64 x 64', ' 64x64', '64x64\n', '9'.repeat(10_000) + 'x64'])('rejects malformed or out-of-range size %# without throwing or coercing', value => {
    expect(() => validateImageSize(value)).not.toThrow();
    expect(validateImageSize(value).length).toBeGreaterThan(0);
    expect(parseImageSize(value)).toBeUndefined();
  });

  it.each([[NaN, 64], [Infinity, 64], [-64, 64], [0, 64], [63, 64], [64, 8193], [1024.2, 768], [64, 64.1]])('refuses numeric %s × %s instead of rounding or clamping', (width, height) => {
    expect(() => formatImageSize(width, height)).toThrow();
    expect(validateImageDimensions(width, height).length).toBeGreaterThan(0);
  });

  it('does not coerce text fields or pretend a custom size is a nearby preset', () => {
    expect(validateImageDimensions('1024', 1024).length).toBeGreaterThan(0);
    expect(formatImageSize(1280, 720)).toBe('1280x720');
    expect(imageSizeRatio('1280x720')).toBe('16:9');
    expect(imageSizeRatio('1919x1080')).toBe('1919:1080');
    expect(imageSizeRatio('1792x768')).toBe('21:9');
    expect(imageSizeRatio('not-a-size')).toBeUndefined();
  });
});
