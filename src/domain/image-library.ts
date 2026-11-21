import type { AssetVersion, Graph, Project, Run } from './types';

export interface LibraryImage {
    asset: AssetVersion;
    projectName: string;
    nodeId?: string;
    nodeName?: string;
    prompt: string;
    used: boolean;
}
export interface LibraryPage { items: LibraryImage[]; total: number; }

/** Include unadopted results and retained images whose original card was removed. */
export function libraryImage(asset: AssetVersion, project: Pick<Project, 'name' | 'graph'>, prompt = ''): LibraryImage {
    const uses = project.graph.nodes.filter(node => node.selectedAssetVersionId === asset.id);
    const node = project.graph.nodes.find(node => node.id === asset.nodeId) ?? uses[0];
    return { asset, projectName: project.name, nodeId: node?.id, nodeName: node?.name, prompt, used: uses.length > 0 };
}
export function localLibrary(assets: AssetVersion[], runs: Run[], graph: Graph, name: string): LibraryImage[] {
    const prompts = new Map(runs.map(run => [run.id, run.inputSnapshot.prompt]));
    return assets.map(asset => libraryImage(asset, { name, graph }, prompts.get(asset.runId ?? '') ?? ''))
        .sort((a, b) => b.asset.createdAt.localeCompare(a.asset.createdAt) || b.asset.id.localeCompare(a.asset.id));
}
export function filterLibrary(items: LibraryImage[], query: string, since = ''): LibraryImage[] {
    const text = query.trim().toLocaleLowerCase();
    return items.filter(item => (!since || item.asset.createdAt >= since) && (!text ||
        [item.asset.name, item.projectName, item.prompt].some(value => value.toLocaleLowerCase().includes(text))));
}
