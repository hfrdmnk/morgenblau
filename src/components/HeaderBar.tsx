export function HeaderBar() {
	return (
		<nav className="fixed top-0 inset-x-0 h-14 z-30 bg-bg-page flex items-center justify-between px-4">
			<div className="flex items-center gap-6">
				<span className="text-text-tertiary text-sm font-medium">Discover</span>
				<span className="text-text-tertiary text-sm font-medium">Consume</span>
				<span className="text-text-tertiary text-sm font-medium">Create</span>
			</div>

			<div className="flex items-center gap-4">
				<span className="text-text-tertiary text-sm">12:00</span>
				<span className="text-text-tertiary text-sm">Messages</span>
			</div>

			<div className="flex items-center gap-4">
				<div className="w-6 h-6 rounded-full bg-text-tertiary/20"></div>
				<div className="w-6 h-6 rounded-full bg-text-tertiary/20"></div>
			</div>
		</nav>
	)
}
