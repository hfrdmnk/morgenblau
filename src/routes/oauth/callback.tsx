import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { useEffect } from 'react';
import { useAuth } from '../../lib/auth-context';

export const Route = createFileRoute('/oauth/callback')({
	component: OAuthCallback
});

function OAuthCallback() {
	const { status } = useAuth();
	const navigate = useNavigate();

	useEffect(() => {
		if (status === 'authenticated') {
			navigate({ to: '/home' });
		} else if (status === 'unauthenticated') {
			navigate({ to: '/' });
		}
	}, [status, navigate]);

	return null;
}
