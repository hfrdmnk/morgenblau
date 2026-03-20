import type { BrowserOAuthClient } from '@atproto/oauth-client-browser';
import { buildAtprotoLoopbackClientMetadata } from '@atproto/oauth-types';
import { createDefaultHandleResolver } from './handle-resolver';

export type Session = NonNullable<Awaited<ReturnType<BrowserOAuthClient['init']>>>['session'];

let client: BrowserOAuthClient | null = null;
let initPromise: Promise<void> | null = null;
let currentSession: Session | null = null;

async function ensureClient(): Promise<BrowserOAuthClient> {
	if (client) return client;

	const { BrowserOAuthClient } = await import('@atproto/oauth-client-browser');
	const handleResolver = createDefaultHandleResolver();

	if (import.meta.env.DEV) {
		client = new BrowserOAuthClient({
			clientMetadata: buildAtprotoLoopbackClientMetadata({
				scope: 'atproto transition:generic',
				redirect_uris: ['http://127.0.0.1:3000/oauth/callback']
			}),
			handleResolver
		});
	} else {
		client = await BrowserOAuthClient.load({
			clientId: 'https://morgen.blue/client-metadata.json',
			handleResolver
		});
	}

	return client;
}

/**
 * Initialize the auth client and restore any existing session.
 * Safe to call multiple times — only runs client.init() once per page load.
 */
export async function initAuth(): Promise<Session | null> {
	if (typeof window === 'undefined') return null;

	if (!initPromise) {
		initPromise = ensureClient()
			.then(async (c) => {
				const result = await c.init();
				currentSession = result?.session ?? null;
			})
			.catch((err) => {
				initPromise = null;
				throw err;
			});
	}

	await initPromise;
	return currentSession;
}

/**
 * Start the OAuth sign-in flow via redirect. The browser navigates to the
 * ATProto authorization server — the returned promise never resolves.
 */
export async function signIn(handle: string): Promise<never> {
	const c = await ensureClient();
	return c.signInRedirect(handle);
}

export async function signOut(): Promise<void> {
	if (!currentSession || !client) return;

	const sub = currentSession.sub;
	currentSession = null;
	initPromise = null;

	await client.revoke(sub);
}
