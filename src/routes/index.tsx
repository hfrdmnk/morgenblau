import { createFileRoute } from '@tanstack/react-router'
import { ClientOnly } from '@tanstack/react-router'
import { LandingAnimation } from '../components/LandingAnimation'

export const Route = createFileRoute('/')({
	component: LandingPage,
})

function LandingPage() {
	return (
		<ClientOnly fallback={null}>
			<LandingAnimation />
		</ClientOnly>
	)
}
