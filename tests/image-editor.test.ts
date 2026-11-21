import { describe, expect, it } from 'vitest';
import { createNode, DEFAULT_PARAMS, resolveInput } from '../src/domain/graph';
import type { CanvasEdge, Graph, Project } from '../src/domain/types';
import { findOpenPosition, getEditorInputs, makeInputIndependent, PLACEMENT_CARD_SIZE } from '../src/lib/image-editor';

const projectId = 'image-editor-test';
const project = (graph: Graph): Project => ({ id: projectId, name: 'editor', schemaVersion: 1, graphRevision: 1, layoutRevision: 1, createdAt: '', updatedAt: '', graph });
const edge = (sourceNodeId: string, targetNodeId: string, targetPort: string): CanvasEdge => ({ id: `${sourceNodeId}-${targetNodeId}-${targetPort}`, sourceNodeId, sourcePort: 'out', targetNodeId, targetPort });

describe('image editor effective inputs', () => {
    it('falls back from an empty connected params node to local params, matching execution before and after detaching', () => {
        const source = createNode('params', projectId);
        delete source.config.params;
        const image = createNode('generate', projectId);
        image.config.text = 'A quiet tabletop';
        image.config.params = { size: '1536x1024', quality: 'high', n: 3 };
        const graph: Graph = { nodes: [source, image], edges: [edge(source.id, image.id, 'params')] };
        const before = resolveInput(project(graph), image.id, [], 'mock');
        expect(before.params).toEqual(image.config.params);
        expect(getEditorInputs(graph, image).params).toEqual(before.params);

        const detached = makeInputIndependent(graph, image.id, 'params');
        const savedImage = detached.nodes.find(node => node.id === image.id)!;
        expect(getEditorInputs(detached, savedImage).params).toEqual(before.params);
        expect(resolveInput(project(detached), image.id, [], 'mock').params).toEqual(before.params);
        expect(detached.edges).toHaveLength(0);
        expect(source.config.params).toBeUndefined();

        delete image.config.params;
        expect(getEditorInputs(graph, image).params).toEqual(DEFAULT_PARAMS);
        expect(resolveInput(project(graph), image.id, [], 'mock').params).toEqual(DEFAULT_PARAMS);
    });

    it('detaches only the chosen target input while preserving shared sources and another branch', () => {
        const prompt = createNode('text', projectId);
        prompt.config.text = 'Shared prompt';
        const params = createNode('params', projectId);
        params.config.params = { size: '1024x1536', quality: 'low', n: 2 };
        const target = createNode('generate', projectId);
        const other = createNode('generate', projectId);
        const graph: Graph = { nodes: [prompt, params, target, other], edges: [edge(prompt.id, target.id, 'prompt'), edge(params.id, target.id, 'params'), edge(prompt.id, other.id, 'prompt'), edge(params.id, other.id, 'params')] };
        const original = structuredClone(graph);
        const otherBefore = resolveInput(project(graph), other.id, [], 'mock');

        const detachedPrompt = makeInputIndependent(graph, target.id, 'prompt');
        expect(detachedPrompt.edges).toEqual(graph.edges.filter(item => item.id !== graph.edges[0].id));
        expect(detachedPrompt.nodes.find(node => node.id === target.id)!.config.text).toBe('Shared prompt');
        const detachedParams = makeInputIndependent(detachedPrompt, target.id, 'params');
        expect(detachedParams.edges).toEqual(graph.edges.slice(2));
        expect(detachedParams.nodes[0]).toBe(prompt);
        expect(detachedParams.nodes[1]).toBe(params);
        expect(detachedParams.nodes[3]).toBe(other);
        expect(graph).toEqual(original);
        expect(resolveInput(project(detachedParams), other.id, [], 'mock')).toEqual(otherBefore);
        expect(resolveInput(project(detachedParams), target.id, [], 'mock').params).toEqual(params.config.params);
        expect(detachedParams.nodes[2].config.params).not.toBe(params.config.params);
    });

    it('keeps an empty connected prompt authoritative instead of falling back to a local draft', () => {
        const prompt = createNode('text', projectId);
        prompt.config.text = '';
        const image = createNode('generate', projectId);
        image.config.text = 'Local draft must stay inactive';
        const graph: Graph = { nodes: [prompt, image], edges: [edge(prompt.id, image.id, 'prompt')] };
        expect(getEditorInputs(graph, image).prompt).toBe('');
        expect(() => resolveInput(project(graph), image.id, [], 'mock')).toThrow('非空');
        const detached = makeInputIndependent(graph, image.id, 'prompt');
        expect(detached.nodes[1].config.text).toBe('');
        expect(() => resolveInput(project(detached), image.id, [], 'mock')).toThrow('非空');
    });
});

describe('new image placement', () => {
    it('avoids the measured conservative rectangle and leaves the existing layout untouched', () => {
        const existing = createNode('edit', projectId, { x: 260, y: 160 });
        const nodes = [existing];
        const before = structuredClone(nodes);
        const measured = { [existing.id]: { width: 400, height: 600 } };
        const point = findOpenPosition(nodes, { x: 280, y: 180 }, measured);
        const { width, height } = PLACEMENT_CARD_SIZE;
        expect(point.x + width <= 260 || point.x >= 660 || point.y + height <= 160 || point.y >= 760).toBe(true);
        expect(nodes).toEqual(before);
        expect(findOpenPosition(nodes, { x: 1000, y: 1000 }, measured)).toEqual({ x: 1000, y: 1000 });
    });

    it('keeps repeated additions at the same target clear of every previously placed card', () => {
        const nodes = [createNode('generate', projectId, { x: 260, y: 160 })];
        const firstPosition = { ...nodes[0].position };
        const { width, height } = PLACEMENT_CARD_SIZE;
        for (let count = 0; count < 24; count++) {
            const point = findOpenPosition(nodes, { x: 260, y: 160 });
            for (const node of nodes) {
                expect(point.x + width <= node.position.x || point.x >= node.position.x + width || point.y + height <= node.position.y || point.y >= node.position.y + height).toBe(true);
            }
            nodes.push(createNode('image', projectId, point));
        }
        expect(nodes[0].position).toEqual(firstPosition);
    });
});
