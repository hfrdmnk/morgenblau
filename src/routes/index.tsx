import { createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/')({
	component: HomePage,
})

function HomePage() {
	return (
		<>
			{/* Fixed sub-header (calendar + filters) */}
			<div className="fixed top-14 left-2 right-2 z-20 rounded-t-2xl bg-bg-front-1 px-6 py-4">
				<div className="flex items-center justify-between">
					<div className="flex items-center gap-4">
						<span className="text-text-secondary text-sm">Mon 3</span>
						<span className="text-text-secondary text-sm">Tue 4</span>
						<span className="text-text-primary text-sm font-semibold">Wed 5</span>
						<span className="text-text-secondary text-sm">Thu 6</span>
						<span className="text-text-secondary text-sm">Fri 7</span>
					</div>
					<div className="flex items-center gap-2">
						<button className="px-3 py-1 text-xs rounded-full bg-bg-front-2 text-text-primary border border-border-1">
							All
						</button>
						<button className="px-3 py-1 text-xs rounded-full text-text-secondary border border-border-1">
							Events
						</button>
						<button className="px-3 py-1 text-xs rounded-full text-text-secondary border border-border-1">
							Tasks
						</button>
					</div>
				</div>
			</div>

			{/* Content (normal flow, scrolls with page) */}
			<div className="flex flex-col gap-3">
				{Array.from({ length: 20 }, (_, i) => (
					<div
						key={i}
						className="rounded-xl bg-bg-front-2 border border-border-1 p-4"
					>
						<div className="flex items-center justify-between">
							<div>
								<p className="text-text-primary text-sm font-medium">
									Placeholder item {i + 1}
								</p>
								<p className="text-text-secondary text-xs mt-1">
									Some description text here
								</p>
							</div>
							<span className="text-text-secondary text-xs">9:00 AM</span>
						</div>
					</div>
				))}
			</div>
		</>
	)
}
