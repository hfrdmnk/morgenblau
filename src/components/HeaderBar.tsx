export function HeaderBar() {
	return (
		<nav className="fixed inset-x-0 top-0 z-30 flex h-14 items-center justify-between bg-background px-4">
			<div className="flex items-center gap-6">
				<span className="text-sm font-medium text-muted-foreground">Discover</span>
				<span className="text-sm font-medium text-muted-foreground">Consume</span>
				<span className="text-sm font-medium text-muted-foreground">Create</span>
			</div>

			<div className="flex items-center gap-4">
				<span className="text-sm text-muted-foreground">12:00</span>
				<span className="text-sm text-muted-foreground">Messages</span>
			</div>

			<div className="flex items-center gap-4">
				<div className="h-6 w-6 rounded-full bg-muted-foreground/20"></div>
				<div className="h-6 w-6 rounded-full bg-muted-foreground/20"></div>
			</div>
		</nav>
	);
}
