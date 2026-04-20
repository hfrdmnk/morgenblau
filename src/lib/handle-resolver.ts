import {
	CompositeHandleResolver,
	DohJsonHandleResolver,
	XrpcHandleResolver,
	type HandleResolver
} from '@atcute/identity-resolver';

const DOH_ENDPOINT = 'https://cloudflare-dns.com/dns-query';
const BLUESKY_APPVIEW = 'https://bsky.social';

export function createDefaultHandleResolver(): HandleResolver {
	return new CompositeHandleResolver({
		methods: {
			dns: new DohJsonHandleResolver({ dohUrl: DOH_ENDPOINT }),
			http: new XrpcHandleResolver({ serviceUrl: BLUESKY_APPVIEW })
		},
		strategy: 'dns-first'
	});
}
