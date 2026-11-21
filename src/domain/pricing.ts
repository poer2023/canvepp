import type { ApiImageSize, AssetVersion, ImageParams, InputSnapshot } from './types';

/**
 * Official reference facts checked on 2026-10-29. These are output prices, NOT
 * complete request ceilings. Input text and (for edits) image tokens are extra.
 * A user-entered price, safety multiplier, or arbitrary future expiry cannot
 * turn this reference table into a proven request upper bound.
 */
export const PRICING_REFERENCE = Object.freeze({
  model: 'gpt-image-1.5',
  verifiedOn: '2026-10-29',
  // Application review lease, not an OpenAI promise that rates stay unchanged.
  reviewExpiresAt: '2026-10-30T15:59:59.999Z',
  status: 'deprecated',
  scheduledShutdownOn: '2026-12-01',
  currency: 'USD',
  textInputPerMillion: 5,
  imageInputPerMillion: 8,
  imageOutputPerMillion: 32,
  sources: Object.freeze({
    model: 'https://developers.openai.com/api/docs/models/gpt-image-1.5',
    generation: 'https://developers.openai.com/api/docs/guides/image-generation#cost-and-latency',
    inputRules: 'https://developers.openai.com/api/docs/guides/images-vision#gpt-image-model-inputs',
    deprecations: 'https://developers.openai.com/api/docs/deprecations#2026-06-02-gpt-image-model-deprecations',
  }),
});

export const REFERENCE_OUTPUT_PRICES = Object.freeze({
  low: Object.freeze({ '1024x1024': 0.009, '1024x1536': 0.013, '1536x1024': 0.013 }),
  medium: Object.freeze({ '1024x1024': 0.034, '1024x1536': 0.05, '1536x1024': 0.05 }),
  high: Object.freeze({ '1024x1024': 0.133, '1024x1536': 0.2, '1536x1024': 0.2 }),
}) satisfies Readonly<Record<ImageParams['quality'], Readonly<Record<ApiImageSize, number>>>>;

export type PricingBlockCode = 'pricing_expired' | 'unsupported_model' | 'missing_reference' | 'missing_upper_bound';
export class PricingUnavailableError extends Error {
  readonly errorClass = 'pricing_unknown';
  readonly code: PricingBlockCode;
  constructor(code: PricingBlockCode, message: string) {
    super(message); this.code = code; this.name = 'PricingUnavailableError';
  }
}

/** Informational output-only subtotal. Never use this function to reserve budget. */
export function referenceOutputSubtotal(snapshot: InputSnapshot): number | undefined {
  if (snapshot.provider !== 'openai' || snapshot.model !== PRICING_REFERENCE.model) return undefined;
  const row = REFERENCE_OUTPUT_PRICES[snapshot.params.quality];
  if (!row || !Object.hasOwn(row, snapshot.params.size)) return undefined;
  const price = row[snapshot.params.size as ApiImageSize];
  return price !== undefined && Number.isInteger(snapshot.params.n) && snapshot.params.n >= 1 && snapshot.params.n <= 4 ? Math.round(price * snapshot.params.n * 1_000_000) / 1_000_000 : undefined;
}

/**
 * This release intentionally refuses all paid estimates. The current reviewed
 * docs provide token rates and output tables, but we have not established a
 * documented tokenizer/overhead ceiling for the exact Images API prompt path,
 * nor a GPT Image 1.5 image-input token ceiling. Vision-model patch/tile rules
 * explicitly do not cover GPT Image generation/editing; legacy GPT Image 1
 * rules cannot be carried over to 1.5. A 32,000-character or 40MP local limit
 * alone does not prove a billable-token upper bound.
 *
 * Keep API requests blocked until a separate, reviewed pricing implementation
 * proves ALL charge components for the exact adapter/model. No fallback model,
 * user price field, or arbitrary multiplier is an acceptable substitute.
 */
export function conservativeEstimatedCost(snapshot: InputSnapshot, asset?: AssetVersion, at = Date.now()): number {
  if (snapshot.provider === 'mock' || snapshot.provider === 'manual' || snapshot.provider === 'codex') return 0;
  if (snapshot.provider !== 'openai' || snapshot.model !== PRICING_REFERENCE.model) {
    throw new PricingUnavailableError('unsupported_model', '当前模型尚无经验证的费用上界，已阻止真实 API 提交。请使用模拟或手动模式。');
  }
  if (!Number.isFinite(at) || at > Date.parse(PRICING_REFERENCE.reviewExpiresAt)) {
    throw new PricingUnavailableError('pricing_expired', '内置价格证据已过复核期限，已阻止真实 API 提交；修改价格日期不能解除此限制。');
  }
  if (snapshot.nodeKind === 'edit' && (!asset || asset.id !== snapshot.sourceAssetVersionId || asset.sha256 !== snapshot.sourceAssetHash)) {
    throw new PricingUnavailableError('missing_reference', '尚未核实编辑主图的不可变版本，无法计算完整费用上界。');
  }
  throw new PricingUnavailableError('missing_upper_bound', snapshot.nodeKind === 'edit'
    ? '尚未证明此模型提示词与参考图的完整计费上界，已阻止真实 API 提交。每张输出参考价不能替代总请求费用；可继续使用模拟或手动模式。'
    : '尚未证明此模型提示词的完整计费上界，已阻止真实 API 提交。每张输出参考价不能替代总请求费用；可继续使用模拟或手动模式。');
}
