import { hostRequestBody } from '@/server/host-request';
import { getStore } from '@/server/store';
import { authorize, checkOrigin } from '@/server/security';
import { assert, publicError } from '@/server/errors';
import { readHostContext, requireHostContext } from '@/server/host-dispatch';
import { openThreadCanvas } from '@/server/thread-canvas';

export const runtime = 'nodejs';
export const dynamic = 'force-dynamic';
const headers = { 'Cache-Control': 'no-store', 'X-Content-Type-Options': 'nosniff' };
export async function POST(request: Request) {
    try {
        checkOrigin(request);
        const store = getStore();
        authorize(store, request);
        const body = await hostRequestBody(request);
        const token = request.headers.get('x-canvepp-host') ?? body.hostContext;
        assert(typeof token === 'string', '请从 Codex 会话打开画布', 403, 'host_disconnected');
        assert(body.projectId === undefined || typeof body.projectId === 'string', '画布标识无效');
        const context = requireHostContext(store, token);
        const detail = openThreadCanvas(store, context.thread_id, body.projectId as string | undefined);
        return Response.json({ context: readHostContext(store, token), detail }, { headers });
    } catch (error) {
        const failure = publicError(error);
        return Response.json(failure, { status: failure.status, headers });
    }
}
