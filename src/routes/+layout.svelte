<script lang="ts">
	import favicon from '$lib/assets/favicon.svg';
	import HeaderBar from '$lib/components/HeaderBar.svelte';
	import './layout.css';

	let { children } = $props();
</script>

<svelte:head><link rel="icon" href={favicon} /></svelte:head>

<div>
	<!-- Fixed card background (just the visual shape) -->
	<div class="fixed top-14 right-2 bottom-2 left-2 -z-10 rounded-2xl bg-bg-front-1"></div>

	<!-- Fixed nav bar -->
	<HeaderBar />

	<!-- Fixed bottom mask (covers area below card) -->
	<div class="fixed inset-x-0 bottom-0 z-10 h-2 bg-bg-page"></div>

	<!-- Scrollable content wrapper (normal document flow) -->
	<main class="px-8 pt-32 pb-4">
		{@render children()}
	</main>
</div>
