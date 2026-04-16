import { createFileRoute } from '@tanstack/react-router';

export const Route = createFileRoute('/_authenticated/home')({
	component: HomePage
});

function HomePage() {
	return (
		<>
			<div className="fixed top-14 right-2 left-2 z-20 rounded-t-2xl bg-card px-6 py-4">
				<div className="flex items-center justify-between">
					<div className="flex items-center gap-4">
						<span className="text-sm text-muted-foreground">Mon 3</span>
						<span className="text-sm text-muted-foreground">Tue 4</span>
						<span className="text-sm font-semibold text-foreground">Wed 5</span>
						<span className="text-sm text-muted-foreground">Thu 6</span>
						<span className="text-sm text-muted-foreground">Fri 7</span>
					</div>
					<div className="flex items-center gap-2">
						<button className="rounded-full border border-border bg-card px-3 py-1 text-xs text-foreground">
							All
						</button>
						<button className="rounded-full border border-border px-3 py-1 text-xs text-muted-foreground">
							Events
						</button>
						<button className="rounded-full border border-border px-3 py-1 text-xs text-muted-foreground">
							Tasks
						</button>
					</div>
				</div>
			</div>

			<div className="flex flex-col gap-3">
				{Array.from({ length: 20 }, (_, i) => (
					<div key={i} className="rounded-xl border border-border bg-card p-4">
						<div className="flex items-center justify-between">
							<div>
								<p className="text-sm font-medium text-foreground">Placeholder item {i + 1}</p>
								<p className="mt-1 text-xs text-muted-foreground">Some description text here</p>
							</div>
							<span className="text-xs text-muted-foreground">9:00 AM</span>
						</div>
					</div>
				))}
			</div>
		</>
	);
}
