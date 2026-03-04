import { useState, useEffect } from 'react'
import { BackgroundShader } from './BackgroundShader'

/* ─────────────────────────────────────────────────────────
 * ANIMATION STORYBOARD (4 stages)
 *
 *    0ms   mount — shader always full opacity; overlay covers it
 *  200ms   reveal — overlay slides up (translateY -100%)       1.0s gentle ease-out
 * 1200ms   frame — clip insets + rounds in one motion          1.8s soft ease-out
 * 2300ms   text fades in                                       0.6s ease-out
 * ───────────────────────────────────────────────────────── */

const TIMING = {
	reveal: 200,
	frame: 1200,
	text: 2300,
}

const REVEAL = {
	duration: 1.0,
	ease: [0.25, 1, 0.36, 1], // gentle ease-out — starts with intention, settles peacefully
}

const FRAME = {
	duration: 1.8,
	ease: [0.22, 1, 0.36, 1], // soft ease-out
	inset: 2, // rem
	radiusTop: 4, // rem
	radiusBottom: 0.5, // rem
}

const TEXT = {
	duration: 0.6,
	ease: [0.25, 1, 0.36, 1],
}

export function LandingAnimation() {
	const [stage, setStage] = useState(0)

	useEffect(() => {
		const t1 = setTimeout(() => setStage(1), TIMING.reveal)
		const t2 = setTimeout(() => setStage(2), TIMING.frame)
		const t3 = setTimeout(() => setStage(3), TIMING.text)
		return () => {
			clearTimeout(t1)
			clearTimeout(t2)
			clearTimeout(t3)
		}
	}, [])

	const clipFull = 'inset(0rem 0rem 0rem 0rem round 0rem 0rem 0rem 0rem)'
	const clipFramed = `inset(${FRAME.inset}rem ${FRAME.inset}rem ${FRAME.inset}rem ${FRAME.inset}rem round ${FRAME.radiusTop}rem ${FRAME.radiusTop}rem ${FRAME.radiusBottom}rem ${FRAME.radiusBottom}rem)`

	const clipPath = stage < 2 ? clipFull : clipFramed

	const frameTrans = `clip-path ${FRAME.duration}s cubic-bezier(${FRAME.ease.join(',')})`
	const transition = stage >= 2 ? frameTrans : 'none'

	return (
		<div className="fixed inset-0 overflow-hidden">
			<div
				style={{
					clipPath,
					transition,
					position: 'absolute',
					inset: 0,
				}}
			>
				<BackgroundShader />
			</div>

			<div
				style={{
					position: 'absolute',
					inset: 0,
					backgroundColor: 'var(--color-bg-page)',
					transform: stage >= 1 ? 'translateY(-100%)' : 'translateY(0)',
					transition: `transform ${REVEAL.duration}s cubic-bezier(${REVEAL.ease.join(',')})`,
				}}
			/>

			<div
				style={{
					opacity: stage >= 3 ? 1 : 0,
					transition: `opacity ${TEXT.duration}s cubic-bezier(${TEXT.ease.join(',')})`,
				}}
				className="absolute inset-0 flex flex-col items-center justify-center"
			>
				<h1
					className="text-4xl font-semibold tracking-tight text-white"
					style={{ textShadow: '0 1px 12px rgba(0,0,0,0.2)' }}
				>
					morgenblau
				</h1>
				<p className="mt-3 text-base text-white/70">
					Login with your Atmosphere account
				</p>
			</div>
		</div>
	)
}
