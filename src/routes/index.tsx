import { ClientOnly, createFileRoute, useNavigate } from '@tanstack/react-router';
import { useEffect } from 'react';
import { LandingAnimation } from '../components/LandingAnimation';
import { useAuth } from '../lib/auth-context';

export const Route = createFileRoute('/')({
	component: LandingPage
});

function LandingPage() {
	const { status } = useAuth();
	const navigate = useNavigate();

	useEffect(() => {
		if (status === 'authenticated') {
			navigate({ to: '/home' });
		}
	}, [status, navigate]);

	return (
		<ClientOnly fallback={null}>
			<LandingAnimation />
		</ClientOnly>
	);
}
