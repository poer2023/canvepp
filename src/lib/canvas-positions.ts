export type CanvasPositions = Readonly<Record<string, Readonly<{ x: number; y: number }>>>;
const EMPTY: CanvasPositions = Object.freeze({});

/** Transient view coordinates. Persistence and history stay with the editor. */
export function createCanvasPositionStore() {
    let snapshot = EMPTY;
    const listeners = new Set<() => void>();
    return {
        getSnapshot: () => snapshot,
        getServerSnapshot: () => EMPTY,
        subscribe: (listener: () => void) => { listeners.add(listener); return () => { listeners.delete(listener); }; },
        publish(positions: CanvasPositions) {
            const ids = Object.keys(positions);
            if (ids.length === Object.keys(snapshot).length && ids.every(id => snapshot[id]?.x === positions[id].x && snapshot[id]?.y === positions[id].y)) return;
            snapshot = ids.length ? Object.fromEntries(ids.map(id => [id, { ...positions[id] }])) : EMPTY;
            for (const listener of listeners) listener();
        },
        clear() {
            if (snapshot === EMPTY) return;
            snapshot = EMPTY;
            for (const listener of listeners) listener();
        },
    };
}
export type CanvasPositionStore = ReturnType<typeof createCanvasPositionStore>;
