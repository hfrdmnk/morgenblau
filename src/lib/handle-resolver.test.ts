import { describe, it, expect, vi } from 'vitest';
import { createMultiStrategyResolver } from './handle-resolver';

describe('createMultiStrategyResolver', () => {
	it('returns the DID from the primary resolver when it succeeds', async () => {
		const primary = { resolve: vi.fn().mockResolvedValue('did:plc:abc123') };
		const fallback = { resolve: vi.fn().mockResolvedValue('did:plc:other') };

		const resolver = createMultiStrategyResolver(primary, fallback);
		const result = await resolver.resolve('alice.example.com');

		expect(result).toBe('did:plc:abc123');
		expect(primary.resolve).toHaveBeenCalledWith('alice.example.com');
		expect(fallback.resolve).not.toHaveBeenCalled();
	});

	it('falls back to secondary resolver when primary returns null', async () => {
		const primary = { resolve: vi.fn().mockResolvedValue(null) };
		const fallback = { resolve: vi.fn().mockResolvedValue('did:plc:bsky456') };

		const resolver = createMultiStrategyResolver(primary, fallback);
		const result = await resolver.resolve('alice.bsky.social');

		expect(result).toBe('did:plc:bsky456');
		expect(primary.resolve).toHaveBeenCalled();
		expect(fallback.resolve).toHaveBeenCalled();
	});

	it('falls back to secondary resolver when primary throws', async () => {
		const primary = { resolve: vi.fn().mockRejectedValue(new Error('DNS failure')) };
		const fallback = { resolve: vi.fn().mockResolvedValue('did:plc:bsky789') };

		const resolver = createMultiStrategyResolver(primary, fallback);
		const result = await resolver.resolve('alice.bsky.social');

		expect(result).toBe('did:plc:bsky789');
	});

	it('returns null when both resolvers fail to resolve', async () => {
		const primary = { resolve: vi.fn().mockResolvedValue(null) };
		const fallback = { resolve: vi.fn().mockResolvedValue(null) };

		const resolver = createMultiStrategyResolver(primary, fallback);
		const result = await resolver.resolve('nonexistent.example.com');

		expect(result).toBeNull();
	});

	it('throws when both resolvers throw', async () => {
		const primary = { resolve: vi.fn().mockRejectedValue(new Error('DNS fail')) };
		const fallback = { resolve: vi.fn().mockRejectedValue(new Error('XRPC fail')) };

		const resolver = createMultiStrategyResolver(primary, fallback);

		await expect(resolver.resolve('broken.example.com')).rejects.toThrow('XRPC fail');
	});
});
