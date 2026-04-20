import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const mockConfigureOAuth = vi.fn();
const mockCreateAuthorizationUrl = vi.fn();
const mockFinalizeAuthorization = vi.fn();
const mockGetSession = vi.fn();
const mockDeleteStoredSession = vi.fn();
const mockSignOut = vi.fn();

vi.mock('@atcute/oauth-browser-client', () => ({
	configureOAuth: mockConfigureOAuth,
	createAuthorizationUrl: mockCreateAuthorizationUrl,
	finalizeAuthorization: mockFinalizeAuthorization,
	getSession: mockGetSession,
	deleteStoredSession: mockDeleteStoredSession,
	OAuthUserAgent: vi.fn().mockImplementation(function () {
		return { signOut: mockSignOut };
	})
}));

vi.mock('@atcute/identity-resolver', () => ({
	CompositeDidDocumentResolver: vi.fn(),
	LocalActorResolver: vi.fn(),
	PlcDidDocumentResolver: vi.fn(),
	WebDidDocumentResolver: vi.fn()
}));

vi.mock('@atcute/lexicons/syntax', () => ({
	isActorIdentifier: (value: unknown) => typeof value === 'string' && value.length > 0
}));

vi.mock('./handle-resolver', () => ({
	createDefaultHandleResolver: vi.fn().mockReturnValue({ resolve: vi.fn() })
}));

const LAST_DID_KEY = 'morgenblau:oauth:last-did';
const DID = 'did:plc:abc123';
const SESSION = {
	info: { sub: DID, aud: 'https://pds.example', server: {} },
	token: {},
	dpopKey: {}
};

let auth: typeof import('./auth');
let storage: Record<string, string>;

function stubLocalStorage() {
	storage = {};
	vi.stubGlobal('localStorage', {
		getItem: (k: string) => storage[k] ?? null,
		setItem: (k: string, v: string) => {
			storage[k] = v;
		},
		removeItem: (k: string) => {
			delete storage[k];
		}
	});
}

beforeEach(async () => {
	vi.stubGlobal('window', {});
	vi.stubEnv('VITE_OAUTH_CLIENT_ID', 'http://localhost?redirect_uri=x&scope=y');
	vi.stubEnv('VITE_OAUTH_REDIRECT_URI', 'http://127.0.0.1:3000/oauth/callback');
	vi.stubEnv('VITE_OAUTH_SCOPE', 'atproto');
	stubLocalStorage();
	vi.resetModules();
	mockConfigureOAuth.mockReset();
	mockCreateAuthorizationUrl.mockReset();
	mockFinalizeAuthorization.mockReset();
	mockGetSession.mockReset();
	mockDeleteStoredSession.mockReset();
	mockSignOut.mockReset();
	auth = await import('./auth');
});

afterEach(() => {
	vi.unstubAllGlobals();
	vi.unstubAllEnvs();
});

describe('initAuth', () => {
	it('returns null when no DID is stored', async () => {
		const session = await auth.initAuth();

		expect(session).toBeNull();
		expect(mockGetSession).not.toHaveBeenCalled();
	});

	it('resumes the session when a DID is stored', async () => {
		storage[LAST_DID_KEY] = DID;
		mockGetSession.mockResolvedValue(SESSION);

		const session = await auth.initAuth();

		expect(mockGetSession).toHaveBeenCalledWith(DID, { allowStale: true });
		expect(session).toEqual(SESSION);
	});

	it('clears the stored DID when the session cannot be resumed', async () => {
		storage[LAST_DID_KEY] = DID;
		mockGetSession.mockRejectedValue(new Error('revoked'));

		const session = await auth.initAuth();

		expect(session).toBeNull();
		expect(storage[LAST_DID_KEY]).toBeUndefined();
	});

	it('deduplicates concurrent calls', async () => {
		storage[LAST_DID_KEY] = DID;
		mockGetSession.mockResolvedValue(SESSION);

		await Promise.all([auth.initAuth(), auth.initAuth()]);

		expect(mockGetSession).toHaveBeenCalledTimes(1);
	});
});

describe('finalizeCallback', () => {
	it('stores the DID and caches the session', async () => {
		vi.stubGlobal('location', { hash: '#code=xyz', pathname: '/oauth/callback', search: '' });
		vi.stubGlobal('history', { replaceState: vi.fn() });
		mockFinalizeAuthorization.mockResolvedValue({ session: SESSION });

		const session = await auth.finalizeCallback();

		expect(session).toEqual(SESSION);
		expect(storage[LAST_DID_KEY]).toBe(DID);

		// Subsequent initAuth returns the cached session without hitting getSession.
		const resumed = await auth.initAuth();
		expect(resumed).toEqual(SESSION);
		expect(mockGetSession).not.toHaveBeenCalled();
	});
});

describe('signOut', () => {
	it('revokes the session and clears stored state', async () => {
		storage[LAST_DID_KEY] = DID;
		mockGetSession.mockResolvedValue(SESSION);
		await auth.initAuth();

		await auth.signOut();

		expect(mockSignOut).toHaveBeenCalledTimes(1);
		expect(storage[LAST_DID_KEY]).toBeUndefined();
	});

	it('falls back to deleteStoredSession when revoke fails', async () => {
		storage[LAST_DID_KEY] = DID;
		mockGetSession.mockResolvedValue(SESSION);
		await auth.initAuth();
		mockSignOut.mockRejectedValueOnce(new Error('network'));

		await auth.signOut();

		expect(mockDeleteStoredSession).toHaveBeenCalledWith(DID);
		expect(storage[LAST_DID_KEY]).toBeUndefined();
	});

	it('is a no-op when not authenticated', async () => {
		await auth.signOut();

		expect(mockSignOut).not.toHaveBeenCalled();
		expect(mockDeleteStoredSession).not.toHaveBeenCalled();
	});
});
