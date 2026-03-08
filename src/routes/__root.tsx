import { createRootRoute, HeadContent, Outlet, Scripts } from '@tanstack/react-router';
import { DialRoot } from 'dialkit';
import 'dialkit/styles.css';
import { AuthProvider } from '../lib/auth-context';
import appCss from '../styles.css?url';

export const Route = createRootRoute({
	head: () => ({
		links: [{ rel: 'stylesheet', href: appCss }],
		meta: [
			{ charSet: 'utf-8' },
			{ name: 'viewport', content: 'width=device-width, initial-scale=1' }
		]
	}),
	component: RootLayout
});

function RootLayout() {
	return (
		<html lang="en">
			<head>
				<HeadContent />
			</head>
			<body>
				<AuthProvider>
					<Outlet />
				</AuthProvider>
				<DialRoot />
				<Scripts />
			</body>
		</html>
	);
}
