export default {
	async scheduled(_controller, env) {
		const res = await fetch('https://api.github.com/repos/hfrdmnk/morgenblau/actions/workflows/site-refresh.yml/dispatches', {
			method: 'POST',
			headers: {
				Authorization: `Bearer ${env.GITHUB_TOKEN}`,
				Accept: 'application/vnd.github+json',
				// GitHub rejects API requests without a User-Agent, and Workers send none.
				'User-Agent': 'morgenblau-site-refresh',
			},
			body: JSON.stringify({ ref: 'main' }),
		});
		if (!res.ok) throw new Error(`Dispatching site-refresh.yml failed: ${res.status} ${await res.text()}`);
	},
};
