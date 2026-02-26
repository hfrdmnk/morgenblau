import { createFileRoute } from '@tanstack/react-router'
import { ClientOnly } from '@tanstack/react-router'
import { BackgroundShader } from '../components/BackgroundShader'

export const Route = createFileRoute('/')({
	component: LandingPage,
})

function LandingPage() {
	return (
		<ClientOnly fallback={null}>
			<BackgroundShader />
		</ClientOnly>
	)
}
