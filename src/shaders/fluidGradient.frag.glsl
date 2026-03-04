precision highp float;

uniform float uTime;
uniform vec3  uColor1;
uniform vec3  uColor2;
uniform vec3  uColor3;
uniform vec3  uColor4;
uniform float uSpeed;
uniform float uScale;
uniform float uWarp;
uniform float uRevealDuration;

varying vec2 vUv;

// fBm — 2 octaves
float fbm(vec2 p) {
	float v = 0.0;
	v += 0.5 * snoise(p);
	v += 0.25 * snoise(p * 2.0 + 3.7);
	return v;
}

// Film grain
float grain(vec2 co, float t) {
	return fract(sin(dot(co + t, vec2(12.9898, 78.233))) * 43758.5453);
}

void main() {
	float t = uTime * uSpeed;

	float revealT = clamp(uTime / uRevealDuration, 0.0, 1.0);
	revealT = 1.0 - pow(1.0 - revealT, 3.0); // easeOutCubic

	// Stretch coordinates for elongated flowing shapes
	vec2 st = vUv * uScale;
	st.y *= 0.7;

	// Multi-layer domain warp for smooth sweeping curves
	vec2 q = vec2(
		fbm(st + vec2(0.0, 0.0)),
		fbm(st + vec2(5.2, 1.3))
	);

	vec2 r = vec2(
		fbm(st + uWarp * q + vec2(1.7, 9.2) + t * 0.8),
		fbm(st + uWarp * q + vec2(8.3, 2.8) + t * 0.6)
	);

	// Final noise from double-warped coordinates
	float n = fbm(st + uWarp * r) * 0.5 + 0.5;

	// Secondary field for lavender accent
	float n2 = fbm(st * 1.2 + r * 0.5 + vec2(t * 0.3, 4.0)) * 0.5 + 0.5;

	// Boost contrast — push darks darker, brights brighter
	float shaped = pow(n, 1.4);

	// Color mapping: dark base → blue → bright cyan
	vec3 color = mix(uColor2, uColor1, smoothstep(0.0, 0.45, shaped));
	color = mix(color, uColor3, smoothstep(0.4, 0.85, shaped));

	// Subtle lavender in mid-tones only
	float lavMask = smoothstep(0.3, 0.5, n2) * smoothstep(0.7, 0.5, n2);
	color = mix(color, uColor4, lavMask * 0.2);

	// Subtle film grain
	float g = grain(gl_FragCoord.xy * 0.5, t) * 0.03;
	color += g;

	color = mix(uColor2, color, revealT);

	gl_FragColor = vec4(color, 1.0);
}
