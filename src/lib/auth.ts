import { BrowserOAuthClient } from '@atproto/oauth-client-browser';

export type Session = NonNullable<Awaited<ReturnType<BrowserOAuthClient['init']>>>['session'];

let client: BrowserOAuthClient | null = null;
let initPromise: Promise<void> | null = null;
let currentSession: Session | null = null;

async function ensureClient(): Promise<BrowserOAuthClient> {
	if (client) return client;

	if (import.meta.env.DEV) {
		client = new BrowserOAuthClient({
			clientMetadata: undefined,
			handleResolver: 'https://bsky.social'
		});
	} else {
		client = await BrowserOAuthClient.load({
			clientId: 'https://morgen.blue/client-metadata.json',
			handleResolver: 'https://bsky.social'
		});
	}

	return client;
}

/**
 * Initialize the auth client and restore any existing session.
 * Safe to call multiple times — only runs client.init() once per page load.
 */
export async function initAuth(): Promise<Session | null> {
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
