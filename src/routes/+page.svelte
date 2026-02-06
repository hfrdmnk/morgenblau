<!-- Fixed sub-header (calendar + filters) -->
<div class="fixed top-14 left-2 right-2 z-20 rounded-t-2xl bg-bg-front-1 px-6 py-4">
	<div class="flex items-center justify-between">
		<div class="flex items-center gap-4">
			<span class="text-text-secondary text-sm">Mon 3</span>
			<span class="text-text-secondary text-sm">Tue 4</span>
			<span class="text-text-primary text-sm font-semibold">Wed 5</span>
			<span class="text-text-secondary text-sm">Thu 6</span>
			<span class="text-text-secondary text-sm">Fri 7</span>
		</div>
		<div class="flex items-center gap-2">
			<button class="px-3 py-1 text-xs rounded-full bg-bg-front-2 text-text-primary border border-border-1">All</button>
			<button class="px-3 py-1 text-xs rounded-full text-text-secondary border border-border-1">Events</button>
			<button class="px-3 py-1 text-xs rounded-full text-text-secondary border border-border-1">Tasks</button>
		</div>
	</div>
</div>

<!-- Content (normal flow, scrolls with page) -->
<div class="flex flex-col gap-3">
	{#each Array(20) as _, i}
		<div class="rounded-xl bg-bg-front-2 border border-border-1 p-4">
			<div class="flex items-center justify-between">
				<div>
					<p class="text-text-primary text-sm font-medium">Placeholder item {i + 1}</p>
					<p class="text-text-secondary text-xs mt-1">Some description text here</p>
				</div>
				<span class="text-text-secondary text-xs">9:00 AM</span>
			</div>
		</div>
	{/each}
</div>
