import { api, ApiError } from '@/lib/api';
import { toastMutationError } from '@/lib/mutation-toast';

// Users may paste either "alice.bsky.social" or "@alice.bsky.social".
export function normalizeHandleInput(raw: string): string {
    return raw.trim().replace(/^@/, '');
}

// The backend always sends a message when rejecting a follow; anything without one (network failure, abort) gets the fallback.
export function classifyFollowError(error: unknown): string {
    if (error instanceof ApiError) return error.message;
    return 'Couldn’t follow that handle. Try again?';
}

export type FollowRecord = {
    rkey: string;
    subjectDid: string;
    createdAt: string;
};

// Returns the created rkey, or null (with a toast) on failure so the caller can roll back.
export async function requestFollow(handle: string): Promise<string | null> {
    try {
        const body = await api<{ rkey?: string }>('/api/follows', {
            method: 'POST',
            body: { handle },
        });
        return body?.rkey ?? null;
    } catch (err) {
        toastMutationError(err, "Couldn't follow. Try again.");
        return null;
    }
}

export async function requestUnfollow(rkey: string): Promise<boolean> {
    try {
        await api(`/api/follows/${encodeURIComponent(rkey)}`, {
            method: 'DELETE',
        });
        return true;
    } catch (err) {
        toastMutationError(err, "Couldn't unfollow. Try again.");
        return false;
    }
}

// The UI can only add a followed person when the backend echoes both keys; anything less reads as failure.
export function parseFollowResponse(
    body: { rkey?: string; subjectDid?: string; createdAt?: string } | null | undefined,
): FollowRecord | null {
    if (!body?.rkey || !body.subjectDid) return null;
    return {
        rkey: body.rkey,
        subjectDid: body.subjectDid,
        createdAt: body.createdAt ?? '',
    };
}
