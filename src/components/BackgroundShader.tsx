import { Canvas, useFrame } from '@react-three/fiber'
import { useRef } from 'react'
import * as THREE from 'three'

import fragmentBase from '../shaders/fluidGradient.frag.glsl?raw'
import vertexShader from '../shaders/fluidGradient.vert.glsl?raw'
import noiseGlsl from '../shaders/noise.glsl?raw'

const fragmentShader = noiseGlsl + '\n' + fragmentBase

const uniforms = {
	uTime: { value: 0 },
	uColor1: { value: new THREE.Color('#2D3EDC') },
	uColor2: { value: new THREE.Color('#020316') },
	uColor3: { value: new THREE.Color('#93E6FF') },
	uColor4: { value: new THREE.Color('#B294FE') },
	uSpeed: { value: 0.01 },
	uScale: { value: 0.6 },
	uWarp: { value: 1.2 },
	uRevealDuration: { value: 2.0 },
}

function FluidPlane() {
	const matRef = useRef<THREE.ShaderMaterial>(null!)

	useFrame((_, delta) => {
		matRef.current.uniforms.uTime.value += delta
	})

	return (
		<mesh>
			<planeGeometry args={[2, 2]} />
			<shaderMaterial
				ref={matRef}
				vertexShader={vertexShader}
				fragmentShader={fragmentShader}
				uniforms={uniforms}
			/>
		</mesh>
	)
}

export function BackgroundShader() {
	return (
		<Canvas
			orthographic
			camera={{ position: [0, 0, 1], zoom: 1 }}
			dpr={1}
			gl={{ alpha: false, antialias: false, powerPreference: 'high-performance' }}
			onCreated={({ gl }) => gl.setClearColor(0x020316)}
			style={{
				position: 'fixed',
				inset: 0,
				zIndex: -50,
				pointerEvents: 'none',
			}}
		>
			<FluidPlane />
		</Canvas>
	)
}
