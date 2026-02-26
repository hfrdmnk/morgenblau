import { ShaderGradientCanvas, ShaderGradient } from '@shadergradient/react'

export function BackgroundShader() {
	return (
		<ShaderGradientCanvas
			style={{
				position: 'fixed',
				inset: 0,
				zIndex: -50,
				pointerEvents: 'none',
			}}
			pixelDensity={1}
			fov={45}
		>
			<ShaderGradient
				animate="on"
				brightness={1.2}
				cAzimuthAngle={180}
				cDistance={3.6}
				cPolarAngle={90}
				cameraZoom={1}
				color1="#00CCFF"
				color2="#308dff"
				color3="#bde1ff"
				envPreset="city"
				grain="off"
				lightType="3d"
				positionX={-1.4}
				positionY={0}
				positionZ={0}
				reflection={0.1}
				rotationX={0}
				rotationY={10}
				rotationZ={50}
				shader="defaults"
				type="waterPlane"
				uAmplitude={1}
				uDensity={1.3}
				uFrequency={5.5}
				uSpeed={0.1}
				uStrength={1}
				uTime={0}
				wireframe={false}
			/>
		</ShaderGradientCanvas>
	)
}
