import { useState, useEffect, useCallback } from 'react'
import { motion } from 'motion/react'
import { useDialKit } from 'dialkit'
import { BackgroundShader } from './BackgroundShader'

/* ─────────────────────────────────────────────────────────
 * ANIMATION STORYBOARD (5 stages)
 *
 *    0ms   mount — shader invisible (opacity 0), no clipping
 *  400ms   shader fades in (opacity 0 → 1)
 * 1200ms   clip-path insets inward with sharp corners
 * 1800ms   border-radius animates from 0 to rounded
 * 2200ms   text fades in (Motion spring)
 * ───────────────────────────────────────────────────────── */

export function LandingAnimation() {
	const [stage, setStage] = useState(0)
	const [resetCount, setResetCount] = useState(0)

	const p = useDialKit(
		'Landing Animation',
		{
			delays: {
				fade: [200, 0, 6000, 50] as [number, number, number, number],
				clip: [1200, 0, 6000, 50] as [number, number, number, number],
				round: [1200, 0, 6000, 50] as [number, number, number, number],
				text: [3000, 0, 6000, 50] as [number, number, number, number],
			},
			transitions: {
				fade: {
					type: 'easing' as const,
					duration: 1,
					ease: [0, 0, 1, 1] as [number, number, number, number],
				},
				clip: {
					type: 'easing' as const,
					duration: 1.5,
					ease: [0.22, 1, 0.36, 1] as [number, number, number, number],
				},
				round: {
					type: 'easing' as const,
					duration: 1.5,
					ease: [0.42, 0, 0.58, 1] as [number, number, number, number],
				},
				text: {
					type: 'spring' as const,
					visualDuration: 0.6,
					bounce: 0,
				},
			},
			clip: {
				_collapsed: true,
				inset: [2, 0, 10, 0.25] as [number, number, number, number],
				radiusTop: [4, 0, 10, 0.25] as [number, number, number, number],
				radiusBottom: [0.5, 0, 4, 0.1] as [number, number, number, number],
			},
			replay: { type: 'action' as const },
		},
		{
			onAction: useCallback(
				(action: string) => {
					if (action === 'replay') {
						setStage(0)
						setResetCount((c) => c + 1)
					}
				},
				[],
			),
		},
	)

	useEffect(() => {
		const t1 = setTimeout(() => setStage(1), p.delays.fade)
		const t2 = setTimeout(() => setStage(2), p.delays.clip)
		const t3 = setTimeout(() => setStage(3), p.delays.round)
		const t4 = setTimeout(() => setStage(4), p.delays.text)
		return () => {
			clearTimeout(t1)
			clearTimeout(t2)
			clearTimeout(t3)
			clearTimeout(t4)
		}
	}, [resetCount, p.delays.fade, p.delays.clip, p.delays.round, p.delays.text])

	const inset = p.clip.inset
	const rTop = p.clip.radiusTop
	const rBottom = p.clip.radiusBottom

	const clipFull = 'inset(0rem 0rem 0rem 0rem round 0rem 0rem 0rem 0rem)'
	const clipSharp = `inset(${inset}rem ${inset}rem ${inset}rem ${inset}rem round 0rem 0rem 0rem 0rem)`
	const clipRounded = `inset(${inset}rem ${inset}rem ${inset}rem ${inset}rem round ${rTop}rem ${rTop}rem ${rBottom}rem ${rBottom}rem)`

	const clipPath = stage < 2 ? clipFull : stage === 2 ? clipSharp : clipRounded

	// Extract CSS transition string from a DialKit TransitionConfig
	const cssEase = (t: any) =>
		t.type === 'easing'
			? `cubic-bezier(${t.ease.join(',')})`
			: 'ease'
	const cssDur = (t: any) =>
		t.type === 'easing' ? t.duration : (t.visualDuration ?? 0.5)

	const fadeTrans = `opacity ${cssDur(p.transitions.fade)}s ${cssEase(p.transitions.fade)}`

	let transition: string
	if (stage < 2) {
		transition = fadeTrans
	} else if (stage === 2) {
		transition = `${fadeTrans}, clip-path ${cssDur(p.transitions.clip)}s ${cssEase(p.transitions.clip)}`
	} else {
		transition = `${fadeTrans}, clip-path ${cssDur(p.transitions.round)}s ${cssEase(p.transitions.round)}`
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
				initial={{ opacity: 0 }}
				animate={{ opacity: stage >= 4 ? 1 : 0 }}
				transition={p.transitions.text as any}
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
