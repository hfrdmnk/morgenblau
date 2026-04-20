import { createFileRoute, isRedirect, redirect } from '@tanstack/react-router';
import { finalizeCallback } from '../../lib/auth';

export const Route = createFileRoute('/oauth/callback')({
	ssr: false,
	beforeLoad: async () => {
		try {
			await finalizeCallback();
			throw redirect({ to: '/home' });
		} catch (err) {
			if (isRedirect(err)) throw err;
			console.error('OAuth callback failed:', err);
			throw redirect({ to: '/' });
		}
	},
	component: () => null
});
