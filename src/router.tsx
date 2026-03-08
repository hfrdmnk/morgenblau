import { createRouter, Link } from '@tanstack/react-router';
import { routeTree } from './routeTree.gen';

function NotFound() {
	return (
		<div className="flex min-h-screen flex-col items-center justify-center px-4">
			<h1 className="text-primary text-4xl font-bold">404</h1>
			<p className="text-secondary mt-2">Page not found</p>
			<Link to="/" className="mt-6 text-brand hover:underline">
				Back to home
			</Link>
		</div>
	);
}

export function getRouter() {
	const router = createRouter({
		routeTree,
		scrollRestoration: true,
		defaultNotFoundComponent: NotFound
	});
	return router;
}

declare module '@tanstack/react-router' {
	interface Register {
		router: ReturnType<typeof getRouter>;
	}
}
