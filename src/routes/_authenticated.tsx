import { createFileRoute, Outlet } from '@tanstack/react-router';
import { HeaderBar } from '../components/HeaderBar';

export const Route = createFileRoute('/_authenticated')({
	component: AuthenticatedLayout
});

function AuthenticatedLayout() {
	return (
		<div style={{ isolation: 'isolate' }}>
			{/* Fixed card background (just the visual shape) */}
			<div className="fixed top-14 right-2 bottom-2 left-2 -z-10 rounded-2xl bg-bg-front-1" />

			{/* Fixed nav bar */}
			<HeaderBar />

			{/* Fixed bottom mask (covers area below card) */}
			<div className="fixed inset-x-0 bottom-0 z-10 h-2 bg-bg-page" />

			{/* Scrollable content wrapper (normal document flow) */}
			<main className="px-8 pt-32 pb-4">
				<Outlet />
			</main>
		</div>
	);
}
