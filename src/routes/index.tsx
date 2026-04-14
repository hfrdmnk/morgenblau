import { ClientOnly, createFileRoute, redirect } from '@tanstack/react-router';
import { LoginPage } from '../components/LoginPage';
import { initAuth } from '../lib/auth';

export const Route = createFileRoute('/')({
	ssr: false,
	beforeLoad: async () => {
		const session = await initAuth();
		if (session) {
			throw redirect({ to: '/home' });
		}
	},
	component: RouteComponent
});

function RouteComponent() {
	return (
		<ClientOnly fallback={null}>
			<LoginPage />
		</ClientOnly>
	);
}
