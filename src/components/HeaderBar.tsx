export function HeaderBar() {
	return (
		<nav className="fixed inset-x-0 top-0 z-30 flex h-14 items-center justify-between bg-bg-page px-4">
			<div className="flex items-center gap-6">
				<span className="text-sm font-medium text-text-tertiary">Discover</span>
				<span className="text-sm font-medium text-text-tertiary">Consume</span>
				<span className="text-sm font-medium text-text-tertiary">Create</span>
			</div>

			<div className="flex items-center gap-4">
				<span className="text-sm text-text-tertiary">12:00</span>
				<span className="text-sm text-text-tertiary">Messages</span>
			</div>

			<div className="flex items-center gap-4">
				<div className="h-6 w-6 rounded-full bg-text-tertiary/20"></div>
				<div className="h-6 w-6 rounded-full bg-text-tertiary/20"></div>
			</div>
		</nav>
	);
}
