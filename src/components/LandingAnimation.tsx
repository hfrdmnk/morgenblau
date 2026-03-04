import { useState, useEffect } from 'react'
import { motion } from 'motion/react'
import { BackgroundShader } from './BackgroundShader'

/* ─────────────────────────────────────────────────────────
 * ANIMATION STORYBOARD (4 stages)
 *
 *    0ms   mount — shader invisible, full-bleed
 *  300ms   shader fades in (opacity 0 → 1)           1.4s expo-out
 * 1400ms   frame — clip insets + rounds in one motion  1.8s soft ease-out
 * 2600ms   text rises in with stagger                  spring + bounce
 * ───────────────────────────────────────────────────────── */

const TIMING = {
	fade: 300,
	frame: 1400,
	text: 2600,
}

const FADE = {
	duration: 1.4,
	ease: [0.16, 1, 0.3, 1], // expo-out
}

const FRAME = {
	duration: 1.8,
	ease: [0.22, 1, 0.36, 1], // soft ease-out
	inset: 2, // rem
	radiusTop: 4, // rem
	radiusBottom: 0.5, // rem
}

const TEXT = {
	spring: { type: 'spring' as const, visualDuration: 0.6, bounce: 0.12 },
	yOffset: 20, // px rise distance
	stagger: 0.1, // seconds between h1 and p
}

export function LandingAnimation() {
	const [stage, setStage] = useState(0)

	useEffect(() => {
		const t1 = setTimeout(() => setStage(1), TIMING.fade)
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

	const fadeTrans = `opacity ${FADE.duration}s cubic-bezier(${FADE.ease.join(',')})`
	const frameTrans = `clip-path ${FRAME.duration}s cubic-bezier(${FRAME.ease.join(',')})`

	const transition = stage < 2 ? fadeTrans : `${fadeTrans}, ${frameTrans}`

	const textVariants = {
		hidden: { opacity: 0, y: TEXT.yOffset },
		visible: { opacity: 1, y: 0 },
	}

	return (
		<div className="fixed inset-0 overflow-hidden">
			<div
				style={{
					opacity: stage >= 1 ? 1 : 0,
					clipPath,
					transition,
					position: 'absolute',
					inset: 0,
				}}
			>
				<BackgroundShader />
			</div>

			<motion.div
				initial="hidden"
				animate={stage >= 3 ? 'visible' : 'hidden'}
				variants={{
					hidden: {},
					visible: { transition: { staggerChildren: TEXT.stagger } },
				}}
				className="absolute inset-0 flex flex-col items-center justify-center"
			>
				<motion.h1
					variants={textVariants}
					transition={TEXT.spring}
					className="text-4xl font-semibold tracking-tight text-white"
					style={{ textShadow: '0 1px 12px rgba(0,0,0,0.2)' }}
				>
					morgenblau
				</motion.h1>
				<motion.p
					variants={textVariants}
					transition={TEXT.spring}
					className="mt-3 text-base text-white/70"
				>
					Login with your Atmosphere account
				</motion.p>
			</motion.div>
		</div>
	)
}
