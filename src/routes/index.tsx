import { ClientOnly, createFileRoute, redirect } from '@tanstack/react-router';
import { LandingAnimation } from '../components/LandingAnimation';
import { initAuth } from '../lib/auth';

export const Route = createFileRoute('/')({
	ssr: false,
	beforeLoad: async () => {
		const session = await initAuth();
		if (session) {
			throw redirect({ to: '/home' });
		}
	},
	component: LandingPage
});

function LandingPage() {
	return (
		<ClientOnly fallback={null}>
			<LandingAnimation />
		</ClientOnly>
	);
}
