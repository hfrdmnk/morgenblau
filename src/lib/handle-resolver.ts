import type { HandleResolver } from '@atproto-labs/handle-resolver';
import { AtprotoDohHandleResolver, XrpcHandleResolver } from '@atproto-labs/handle-resolver';

const DOH_ENDPOINT = 'https://cloudflare-dns.com/dns-query';
const BLUESKY_APPVIEW = 'https://bsky.social';

export function createMultiStrategyResolver(
	primary: HandleResolver,
	fallback: HandleResolver
): HandleResolver {
	return {
		async resolve(handle: string) {
			try {
				const did = await primary.resolve(handle);
				if (did) return did;
			} catch {
				// fall through to fallback
			}
			return fallback.resolve(handle);
		}
	};
}

export function createDefaultHandleResolver(): HandleResolver {
	const doh = new AtprotoDohHandleResolver({ dohEndpoint: DOH_ENDPOINT });
	const xrpc = new XrpcHandleResolver(BLUESKY_APPVIEW);
	return createMultiStrategyResolver(doh, xrpc);
}
