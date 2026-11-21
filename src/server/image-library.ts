import type { AssetVersion, Project } from '../domain/types';
import { libraryImage, type LibraryPage } from '../domain/image-library';
import type { CanvasStore } from './store';
import { assert } from './errors';

/** Read-only index of images already stored by Canvepp, never chat scraping. */
export function searchLibrary(store: CanvasStore, params: URLSearchParams): LibraryPage {
    const query = (params.get('q') ?? '').trim();
    const since = params.get('since') ?? '';
    const offset = Number(params.get('offset') ?? 0), limit = Number(params.get('limit') ?? 60);
    assert(query.length <= 300, '搜索内容不能超过 300 字');
    assert(!since || Number.isFinite(Date.parse(since)), '日期无效');
    assert(Number.isSafeInteger(offset) && offset >= 0 && Number.isSafeInteger(limit) && limit >= 1 && limit <= 100, '分页参数无效');
    const conditions: string[] = [], values: string[] = [];
    if (query) {
        conditions.push("(json_extract(a.body,'$.name') LIKE ? ESCAPE '\\' OR json_extract(p.body,'$.name') LIKE ? ESCAPE '\\' OR json_extract(r.body,'$.inputSnapshot.prompt') LIKE ? ESCAPE '\\')");
        const escaped = `%${query.replace(/[\\%_]/g, '\\$&')}%`;
        values.push(escaped, escaped, escaped);
    }
    if (since) { conditions.push("json_extract(a.body,'$.createdAt') >= ?"); values.push(new Date(since).toISOString()); }
    const from = `FROM assets a JOIN projects p ON a.project_id=p.id LEFT JOIN runs r ON r.id=json_extract(a.body,'$.runId') AND r.project_id=a.project_id ${conditions.length ? `WHERE ${conditions.join(' AND ')}` : ''}`;
    return store.db.transaction(() => {
        const { total } = store.db.prepare(`SELECT COUNT(*) AS total ${from}`).get(...values) as { total: number };
        const rows = store.db.prepare(`SELECT a.body AS asset, p.body AS project, json_extract(r.body,'$.inputSnapshot.prompt') AS prompt ${from} ORDER BY json_extract(a.body,'$.createdAt') DESC,a.id DESC LIMIT ? OFFSET ?`).all(...values, limit, offset) as { asset: string; project: string; prompt: string | null }[];
        return { total, items: rows.map(row => libraryImage(JSON.parse(row.asset) as AssetVersion, JSON.parse(row.project) as Project, row.prompt ?? '')) };
    })();
}
