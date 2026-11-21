import { describe, expect, it, vi } from 'vitest';
import { createCanvasPositionStore } from '../src/lib/canvas-positions';

describe('transient canvas coordinates', () => {
 it('returns stable snapshots and notifies only changed coordinates', () => {
  const s=createCanvasPositionStore(), listener=vi.fn();s.subscribe(listener);
  expect(s.getSnapshot()).toBe(s.getSnapshot());expect(s.getServerSnapshot()).toBe(s.getServerSnapshot());
  s.publish({a:{x:1,y:2}});const snapshot=s.getSnapshot();
  s.publish({a:{x:1,y:2}});expect(s.getSnapshot()).toBe(snapshot);expect(listener).toHaveBeenCalledTimes(1);
  s.publish({a:{x:2,y:2}});expect(listener).toHaveBeenCalledTimes(2);expect(snapshot.a.x).toBe(1);
 });
 it('takes its own copy and replaces gestures without retaining unrelated IDs', () => {
  const s=createCanvasPositionStore(), positions={a:{x:10,y:20},b:{x:30,y:40}};
  s.publish(positions);positions.a.x=999;expect(s.getSnapshot().a.x).toBe(10);
  s.publish({b:{x:35,y:45}});expect(s.getSnapshot().a).toBeUndefined();expect(s.getSnapshot().b.x).toBe(35);
 });
 it('can clear after canonical commit and safely unsubscribe', () => {
  const s=createCanvasPositionStore(), listener=vi.fn(), unsubscribe=s.subscribe(listener);
  s.publish({a:{x:4,y:6}});s.clear();const empty=s.getSnapshot();s.clear();
  expect(listener).toHaveBeenCalledTimes(2);expect(empty).toBe(s.getServerSnapshot());
  unsubscribe();s.publish({a:{x:8,y:9}});expect(listener).toHaveBeenCalledTimes(2);
 });
});
