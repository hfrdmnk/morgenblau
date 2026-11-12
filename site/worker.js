export default {
	// Static assets answer every request that matches a file; this only sees the rest.
	fetch() {
		return new Response('Not found', { status: 404 });
	},

	async scheduled(_controller, env) {
		const res = await fetch(env.DEPLOY_HOOK_URL, { method: 'POST' });
		if (!res.ok) throw new Error(`Triggering the site build failed: ${res.status} ${await res.text()}`);
	},
};
