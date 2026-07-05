export type ShareOutcome = 'reauth' | 'failed' | 'ok';

// A pre-scope session unsharing a standardfeed share gets a 403 with
// code 'reauth_required'; everything non-2xx else is a generic failure.
export function classifyShareResponse(
    status: number,
    payload: { code?: string } | null,
): ShareOutcome {
    if (status === 403 && payload?.code === 'reauth_required') return 'reauth';
    if (status >= 200 && status < 300) return 'ok';
    return 'failed';
}
