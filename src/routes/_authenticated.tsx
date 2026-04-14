import { createFileRoute, Outlet, redirect } from '@tanstack/react-router';
import { HeaderBar } from '../components/HeaderBar';
import { Window } from '../components/Window';
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
			<HeaderBar />
			<Window>
				<main className="h-full overflow-y-auto px-8 pt-16 pb-4">
					<Outlet />
				</main>
			</Window>
		</div>
	);
}
