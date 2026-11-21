import type { ImageSize } from './types';

export const IMAGE_SIZE_LIMITS = Object.freeze({ minSide: 64, maxSide: 8192, maxPixels: 32_000_000 });
export interface ImageDimensions { width: number; height: number; }
export interface ImageSizePreset extends Readonly<ImageDimensions> { readonly ratio: string; readonly label: string; readonly size: ImageSize; }

/** Validate pixels without coercion, rounding, clamping or swapping dimensions. */
export function validateImageDimensions(width: unknown, height: unknown): string[] {
  const errors: string[] = [];
  const validSide = (value: unknown): value is number => typeof value === 'number' && Number.isInteger(value) && value >= IMAGE_SIZE_LIMITS.minSide && value <= IMAGE_SIZE_LIMITS.maxSide;
  if (!validSide(width)) errors.push('图片宽度须为 64–8192 像素的正整数');
  if (!validSide(height)) errors.push('图片高度须为 64–8192 像素的正整数');
  if (validSide(width) && validSide(height) && width * height > IMAGE_SIZE_LIMITS.maxPixels) errors.push('图片总像素不能超过 3200 万（32 MP）');
  return errors;
}

function readDimensions(value: unknown): ImageDimensions | undefined {
  if (typeof value !== 'string' || !/^[1-9][0-9]{0,3}x[1-9][0-9]{0,3}$/.test(value)) return undefined;
  const [width, height] = value.split('x').map(Number);
  return { width, height };
}

/** A canonical lowercase WxH string is the persisted representation. */
export function validateImageSize(value: unknown): string[] {
  const dimensions = readDimensions(value);
  return dimensions ? validateImageDimensions(dimensions.width, dimensions.height) : ['图片尺寸须使用整数宽x高格式，例如 1920x1080'];
}

/** Invalid input returns undefined; callers can show validateImageSize errors. */
export function parseImageSize(value: unknown): ImageDimensions | undefined {
  const dimensions = readDimensions(value);
  return dimensions && !validateImageDimensions(dimensions.width, dimensions.height).length ? dimensions : undefined;
}

/** Format validated numbers only. Invalid values fail instead of being adjusted. */
export function formatImageSize(width: number, height: number): ImageSize {
  const errors = validateImageDimensions(width, height);
  if (errors.length) throw new Error(errors.join('；'));
  return `${width}x${height}`;
}

export const IMAGE_SIZE_PRESETS: readonly ImageSizePreset[] = Object.freeze(([
  ['1:1', 1024, 1024],
  ['4:3', 1536, 1152],
  ['3:4', 1152, 1536],
  ['3:2', 1536, 1024],
  ['2:3', 1024, 1536],
  ['16:9', 1920, 1080],
  ['9:16', 1080, 1920],
  ['21:9', 1792, 768],
] as const).map(([ratio, width, height]) => Object.freeze({ ratio, label: ratio, width, height, size: formatImageSize(width, height) })));

/** Preserve familiar preset names; custom pixels display their exact reduced ratio. */
export function imageSizeRatio(value: unknown): string | undefined {
  const dimensions = parseImageSize(value);
  if (!dimensions) return undefined;
  const preset = IMAGE_SIZE_PRESETS.find(preset => preset.size === value);
  if (preset) return preset.ratio;
  let divisor = dimensions.width, remainder = dimensions.height;
  while (remainder) [divisor, remainder] = [remainder, divisor % remainder];
  return `${dimensions.width / divisor}:${dimensions.height / divisor}`;
}
