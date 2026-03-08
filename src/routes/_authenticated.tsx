import { createFileRoute, Outlet, useNavigate } from '@tanstack/react-router';
import { useEffect } from 'react';
import { HeaderBar } from '../components/HeaderBar';
import { useAuth } from '../lib/auth-context';

export const Route = createFileRoute('/_authenticated')({
	component: AuthenticatedLayout
});

function AuthenticatedLayout() {
	const { status } = useAuth();
	const navigate = useNavigate();

	useEffect(() => {
		if (status === 'unauthenticated') {
			navigate({ to: '/' });
		}
	}, [status, navigate]);

	if (status !== 'authenticated') return null;

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
