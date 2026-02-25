import {
	createRootRoute,
	HeadContent,
	Outlet,
	Scripts,
} from '@tanstack/react-router'
import { HeaderBar } from '../components/HeaderBar'
import appCss from '../styles.css?url'

export const Route = createRootRoute({
	head: () => ({
		links: [{ rel: 'stylesheet', href: appCss }],
		meta: [
			{ charSet: 'utf-8' },
			{ name: 'viewport', content: 'width=device-width, initial-scale=1' },
		],
	}),
	component: RootLayout,
})

function RootLayout() {
	return (
		<html lang="en">
			<head>
				<HeadContent />
			</head>
			<body>
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
				<Scripts />
			</body>
		</html>
	)
}
