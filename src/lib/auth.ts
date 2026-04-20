import {
	configureOAuth,
	createAuthorizationUrl,
	deleteStoredSession,
	finalizeAuthorization,
	getSession,
	OAuthUserAgent,
	type Session as AtcuteSession
} from '@atcute/oauth-browser-client';
import {
	CompositeDidDocumentResolver,
	LocalActorResolver,
	PlcDidDocumentResolver,
	WebDidDocumentResolver
} from '@atcute/identity-resolver';
import { isActorIdentifier, type Did } from '@atcute/lexicons/syntax';
import { createDefaultHandleResolver } from './handle-resolver';

export type Session = AtcuteSession;

const LAST_DID_KEY = 'morgenblau:oauth:last-did';

let configured = false;
let initPromise: Promise<Session | null> | null = null;
let currentSession: Session | null = null;

function ensureConfigured(): void {
	if (configured) return;

	configureOAuth({
		metadata: {
			client_id: import.meta.env.VITE_OAUTH_CLIENT_ID,
			redirect_uri: import.meta.env.VITE_OAUTH_REDIRECT_URI
		},
		identityResolver: new LocalActorResolver({
			handleResolver: createDefaultHandleResolver(),
			didDocumentResolver: new CompositeDidDocumentResolver({
				methods: {
					plc: new PlcDidDocumentResolver(),
					web: new WebDidDocumentResolver()
				}
			})
		})
	});

	configured = true;
}

const sleep = (ms: number) => new Promise<void>((resolve) => setTimeout(resolve, ms));

/** Safe to call multiple times — only resolves the stored session once per page load. */
export async function initAuth(): Promise<Session | null> {
	if (typeof window === 'undefined') return null;

	if (!initPromise) {
		initPromise = (async () => {
			ensureConfigured();

			const did = localStorage.getItem(LAST_DID_KEY);
			if (!did) return null;

			try {
				currentSession = await getSession(did as Did, { allowStale: true });
				return currentSession;
			} catch {
				localStorage.removeItem(LAST_DID_KEY);
				currentSession = null;
				return null;
			}
		})().catch((err) => {
			initPromise = null;
			throw err;
		});
	}

	return initPromise;
}

/** Call on the OAuth callback route — parses the fragment and stores the session. */
export async function finalizeCallback(): Promise<Session> {
	ensureConfigured();

	const params = new URLSearchParams(location.hash.slice(1));
	const { session } = await finalizeAuthorization(params);

	history.replaceState(null, '', location.pathname + location.search);

	currentSession = session;
	initPromise = Promise.resolve(session);
	localStorage.setItem(LAST_DID_KEY, session.info.sub);

	return session;
}

/** The returned promise never resolves — the browser navigates away. */
export async function signIn(handle: string): Promise<never> {
	ensureConfigured();

	if (!isActorIdentifier(handle)) {
		throw new Error('Invalid handle');
	}

	const url = await createAuthorizationUrl({
		target: { type: 'account', identifier: handle },
		scope: import.meta.env.VITE_OAUTH_SCOPE
	});

	// Let the browser persist the OAuth state to localStorage before navigating.
	await sleep(200);
	window.location.assign(url);

	return new Promise<never>(() => {});
}

export async function signOut(): Promise<void> {
	if (!currentSession) return;

	const session = currentSession;
	currentSession = null;
	initPromise = null;
	localStorage.removeItem(LAST_DID_KEY);

	try {
		await new OAuthUserAgent(session).signOut();
	} catch {
		deleteStoredSession(session.info.sub);
	}
}
