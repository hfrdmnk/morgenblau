import { createFileRoute } from '@tanstack/react-router';

export const Route = createFileRoute('/_authenticated/home')({
	component: HomePage
});

function HomePage() {
	return (
		<>
			<div className="fixed top-14 right-2 left-2 z-20 rounded-t-2xl bg-bg-front-1 px-6 py-4">
				<div className="flex items-center justify-between">
					<div className="flex items-center gap-4">
						<span className="text-sm text-text-secondary">Mon 3</span>
						<span className="text-sm text-text-secondary">Tue 4</span>
						<span className="text-sm font-semibold text-text-primary">Wed 5</span>
						<span className="text-sm text-text-secondary">Thu 6</span>
						<span className="text-sm text-text-secondary">Fri 7</span>
					</div>
					<div className="flex items-center gap-2">
						<button className="rounded-full border border-border-1 bg-bg-front-2 px-3 py-1 text-xs text-text-primary">
							All
						</button>
						<button className="rounded-full border border-border-1 px-3 py-1 text-xs text-text-secondary">
							Events
						</button>
						<button className="rounded-full border border-border-1 px-3 py-1 text-xs text-text-secondary">
							Tasks
						</button>
					</div>
				</div>
			</div>

			<div className="flex flex-col gap-3">
				{Array.from({ length: 20 }, (_, i) => (
					<div key={i} className="rounded-xl border border-border-1 bg-bg-front-2 p-4">
						<div className="flex items-center justify-between">
							<div>
								<p className="text-sm font-medium text-text-primary">Placeholder item {i + 1}</p>
								<p className="mt-1 text-xs text-text-secondary">Some description text here</p>
							</div>
							<span className="text-xs text-text-secondary">9:00 AM</span>
						</div>
					</div>
				))}
			</div>
		</>
	);
}
