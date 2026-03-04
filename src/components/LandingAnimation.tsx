import { useState, useEffect } from 'react'
import { motion } from 'motion/react'
import { BackgroundShader } from './BackgroundShader'

/* ─────────────────────────────────────────────────────────
 * ANIMATION STORYBOARD
 *
 *    0ms   mount — full-screen shader visible, no clipping
 *  400ms   shader clips inward to window shape
 *          clip-path: inset(0) → inset(2rem round 4rem 4rem 0.5rem 0.5rem)
 * 1200ms   "morgenblau" logo + "Login with your Atmosphere account" fade in
 * ───────────────────────────────────────────────────────── */

const TIMING = {
	clip: 400,
	content: 1200,
} as const

const CLIP = {
	full: 'inset(0rem 0rem 0rem 0rem round 0rem 0rem 0rem 0rem)',
	window: 'inset(2rem 2rem 2rem 2rem round 4rem 4rem 0.5rem 0.5rem)',
} as const

const SPRING = {
	clip: { duration: 0.8, ease: [0.22, 1, 0.36, 1] as const },
	fade: { type: 'spring' as const, duration: 0.6, bounce: 0 },
}

export function LandingAnimation() {
	const [stage, setStage] = useState(0)

	useEffect(() => {
		const t1 = setTimeout(() => setStage(1), TIMING.clip)
		const t2 = setTimeout(() => setStage(2), TIMING.content)
		return () => {
			clearTimeout(t1)
			clearTimeout(t2)
		}
	}, [])

	return (
		<div className="fixed inset-0 overflow-hidden">
			<div
				style={{
					clipPath: stage >= 1 ? CLIP.window : CLIP.full,
					transition: `clip-path ${SPRING.clip.duration}s cubic-bezier(${SPRING.clip.ease.join(',')})`,
					position: 'absolute',
					inset: 0,
				}}
			>
				<BackgroundShader />
			</div>

			<motion.div
				animate={{ opacity: stage >= 2 ? 1 : 0 }}
				transition={SPRING.fade}
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
			</motion.div>
		</div>
	)
}
