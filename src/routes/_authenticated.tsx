import { createFileRoute, Outlet, redirect } from '@tanstack/react-router';
import { HeaderBar } from '../components/HeaderBar';
import { initAuth } from '../lib/auth';

export const Route = createFileRoute('/_authenticated')({
	ssr: false,
	beforeLoad: async () => {
		const session = await initAuth();
		if (!session) {
			throw redirect({ to: '/' });
		}
	},
	component: AuthenticatedLayout
});

function AuthenticatedLayout() {
	return (
		<div style={{ isolation: 'isolate' }}>
			<div className="fixed top-14 right-2 bottom-2 left-2 -z-10 rounded-2xl bg-bg-front-1" />
			<HeaderBar />
			<div className="fixed inset-x-0 bottom-0 z-10 h-2 bg-bg-page" />
			<main className="px-8 pt-32 pb-4">
				<Outlet />
			</main>
		</div>
	);
}
