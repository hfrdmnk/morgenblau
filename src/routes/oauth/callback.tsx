import { createFileRoute, redirect } from '@tanstack/react-router';
import { initAuth } from '../../lib/auth';

export const Route = createFileRoute('/oauth/callback')({
	ssr: false,
	beforeLoad: async () => {
		const session = await initAuth();
		throw redirect({ to: session ? '/home' : '/' });
	},
	component: () => null
});
