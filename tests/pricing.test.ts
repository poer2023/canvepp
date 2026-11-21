import { describe, it, expect } from 'vitest';
import { conservativeEstimatedCost, PRICING_REFERENCE, PricingUnavailableError, referenceOutputSubtotal } from '../src/domain/pricing';
import type { AssetVersion, InputSnapshot } from '../src/domain/types';

const reviewedAt = Date.parse('2026-10-29T12:00:00+08:00');
const snapshot = (patch: Partial<InputSnapshot> = {}): InputSnapshot => ({ nodeId: 'generate', nodeKind: 'generate', configVersion: 1, prompt: 'A simple still life', params: { size: '1024x1024', quality: 'low', n: 1 }, upstreamStale: [], model: 'gpt-image-1.5', provider: 'openai', ...patch });
const asset: AssetVersion = { id: 'reference', projectId: 'p', assetId: 'a', sha256: 'a'.repeat(64), mime: 'image/png', width: 1024, height: 1024, byteSize: 1000, createdAt: '', name: 'image', parentVersionIds: [], source: 'import' };
function blockCode(run: () => number) { try { run(); throw new Error('Expected pricing refusal'); } catch (error) { expect(error).toBeInstanceOf(PricingUnavailableError); return (error as PricingUnavailableError).code; } }

describe('request cost gate', () => {
  it('keeps local mock/manual modes free even after reference data expires', () => {
    for (const provider of ['mock', 'manual']) expect(conservativeEstimatedCost(snapshot({ provider }), undefined, Date.parse('2030-01-01'))).toBe(0);
  });
  it('does not mistake an official output price for the whole request bound', () => {
    const input = snapshot();
    expect(referenceOutputSubtotal(input)).toBe(0.009);
    expect(blockCode(() => conservativeEstimatedCost(input, undefined, reviewedAt))).toBe('missing_upper_bound');
  });
  it('refuses edits even for one bounded valid reference image', () => {
    const input = snapshot({ nodeKind: 'edit', sourceAssetVersionId: asset.id, sourceAssetHash: asset.sha256 });
    expect(blockCode(() => conservativeEstimatedCost(input, asset, reviewedAt))).toBe('missing_upper_bound');
    expect(blockCode(() => conservativeEstimatedCost(input, undefined, reviewedAt))).toBe('missing_reference');
    expect(blockCode(() => conservativeEstimatedCost(input, { ...asset, sha256: 'b'.repeat(64) }, reviewedAt))).toBe('missing_reference');
  });
  it('does not make up token ceilings from character count, image pixels or a safety multiplier', () => {
    const input = snapshot({ prompt: '汉'.repeat(32000), nodeKind: 'edit', sourceAssetVersionId: asset.id, sourceAssetHash: asset.sha256, params: { size: '1536x1024', quality: 'high', n: 4 } });
    expect(referenceOutputSubtotal(input)).toBe(0.8);
    expect(blockCode(() => conservativeEstimatedCost(input, { ...asset, width: 8000, height: 5000 }, reviewedAt))).toBe('missing_upper_bound');
  });
  it('rejects expired evidence, unknown models and providers', () => {
    expect(blockCode(() => conservativeEstimatedCost(snapshot(), undefined, Date.parse(PRICING_REFERENCE.reviewExpiresAt) + 1))).toBe('pricing_expired');
    expect(blockCode(() => conservativeEstimatedCost(snapshot({ model: 'gpt-image-2.5-sunburst' }), undefined, reviewedAt))).toBe('unsupported_model');
    expect(blockCode(() => conservativeEstimatedCost(snapshot({ provider: 'arbitrary' }), undefined, reviewedAt))).toBe('unsupported_model');
  });
  it('locks the reference review deadline independently of editable user budget settings', () => {
    expect(Object.isFrozen(PRICING_REFERENCE)).toBe(true);
    expect(Object.isFrozen(PRICING_REFERENCE.sources)).toBe(true);
    const malicious = { ...snapshot(), pricePerImage: 0.00000001, priceValidUntil: '2099-01-01' };
    expect(blockCode(() => conservativeEstimatedCost(malicious, undefined, reviewedAt))).toBe('missing_upper_bound');
    expect(blockCode(() => conservativeEstimatedCost(malicious, undefined, Date.parse('2027-01-01')))).toBe('pricing_expired');
  });
});
