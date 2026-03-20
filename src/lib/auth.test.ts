import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const mockInit = vi.fn();
const mockSignInRedirect = vi.fn();
const mockRevoke = vi.fn();

vi.mock('@atproto/oauth-client-browser', () => ({
	BrowserOAuthClient: vi.fn().mockImplementation(function () {
		return { init: mockInit, signInRedirect: mockSignInRedirect, revoke: mockRevoke };
	})
}));

vi.mock('@atproto/oauth-types', () => ({
	buildAtprotoLoopbackClientMetadata: vi.fn().mockReturnValue({})
}));

vi.mock('./handle-resolver', () => ({
	createDefaultHandleResolver: vi.fn().mockReturnValue({ resolve: vi.fn() })
}));

let auth: typeof import('./auth');

beforeEach(async () => {
	vi.stubGlobal('window', {});
	vi.resetModules();
	mockInit.mockReset();
	mockSignInRedirect.mockReset();
	mockRevoke.mockReset();
	auth = await import('./auth');
});

afterEach(() => {
	vi.unstubAllGlobals();
});

describe('initAuth', () => {
	it('returns the session when client.init() has one', async () => {
		mockInit.mockResolvedValue({ session: { sub: 'did:plc:abc123' } });

		const session = await auth.initAuth();

		expect(session).toEqual({ sub: 'did:plc:abc123' });
	});

	it('returns null when no existing session', async () => {
		mockInit.mockResolvedValue(undefined);

		const session = await auth.initAuth();

		expect(session).toBeNull();
	});

	it('only calls client.init() once across multiple calls', async () => {
		mockInit.mockResolvedValue({ session: { sub: 'did:plc:abc123' } });

		await auth.initAuth();
		await auth.initAuth();

		expect(mockInit).toHaveBeenCalledTimes(1);
	});
});

describe('signOut', () => {
	it('revokes the session and resets init state', async () => {
		mockInit.mockResolvedValue({ session: { sub: 'did:plc:abc123' } });
		await auth.initAuth();

		await auth.signOut();

		expect(mockRevoke).toHaveBeenCalledWith('did:plc:abc123');

		// After sign-out, initAuth should re-initialize (call init again)
		mockInit.mockResolvedValue(undefined);
		const session = await auth.initAuth();
		expect(session).toBeNull();
		expect(mockInit).toHaveBeenCalledTimes(2);
	});

	it('is a no-op when not authenticated', async () => {
		await auth.signOut();

		expect(mockRevoke).not.toHaveBeenCalled();
	});
});
