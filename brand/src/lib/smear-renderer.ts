import {
    MAX_PATCHES,
    MODES,
    type SmearConfig,
    planScene,
} from './smear-config';
import { generatePalette } from './smear-palette';

const VERTEX = `#version 300 es
void main() {
    vec2 corner = vec2(float((gl_VertexID << 1) & 2), float(gl_VertexID & 2));
    gl_Position = vec4(corner * 2.0 - 1.0, 0.0, 1.0);
}`;

// Shared by both passes. p is the image position in units of the shorter image side, origin at the centre, y pointing down.
const COMMON = `#version 300 es
precision highp float;
precision highp int;

uniform vec2 uImage;
uniform float uMinSide;
uniform int uMode;
uniform vec2 uDir;
uniform vec2 uCenter;
uniform vec4 uNoise;

uint pcg(uint v) {
    uint state = v * 747796405u + 2891336453u;
    uint word = ((state >> ((state >> 28u) + 4u)) ^ state) * 277803737u;
    return (word >> 22u) ^ word;
}

float hash(ivec2 cell) {
    return float(pcg(uint(cell.x) ^ pcg(uint(cell.y) + 0x9e3779b9u))) / 4294967295.0;
}

float vnoise(vec2 x) {
    vec2 i = floor(x);
    vec2 f = fract(x);
    vec2 u = f * f * (3.0 - 2.0 * f);
    ivec2 c = ivec2(i);
    return mix(
        mix(hash(c), hash(c + ivec2(1, 0)), u.x),
        mix(hash(c + ivec2(0, 1)), hash(c + ivec2(1, 1)), u.x),
        u.y
    );
}

float fbm(vec2 x) {
    float sum = 0.0;
    float amplitude = 0.5;
    for (int i = 0; i < 4; i++) {
        sum += amplitude * vnoise(x);
        x = mat2(1.6, 1.2, -1.2, 1.6) * x + 13.7;
        amplitude *= 0.5;
    }
    return sum / 0.9375;
}

vec2 flowDir(vec2 p) {
    if (uMode == 1) {
        vec2 v = p - uCenter;
        return normalize(vec2(-v.y, v.x));
    }
    return uDir;
}

vec2 imagePoint(vec2 px) {
    return vec2(px.x - 0.5 * uImage.x, 0.5 * uImage.y - px.y) / uMinSide;
}
`;

const SCENE = `${COMMON}
uniform vec2 uMargin;
uniform vec4 uPatches[${MAX_PATCHES}];
uniform int uPatchCount;
uniform float uPatchiness;
uniform vec3 uColors[4];
uniform vec3 uLightColor;
uniform vec3 uAccentColor;
uniform vec3 uLightMass;
uniform vec3 uAccentMass;
uniform float uAccentAmount;

out vec4 outColor;

float mass(vec2 p, vec3 m, float stretch, vec2 noiseOffset) {
    vec2 d = flowDir(m.xy);
    vec2 local = p - m.xy;
    vec2 frame = vec2(dot(local, d) / stretch, dot(local, vec2(-d.y, d.x)));
    float edge = length(frame) / m.z + (fbm(p * 2.0 / m.z + noiseOffset) - 0.5) * 0.6;
    return 1.0 - smoothstep(0.0, 1.0, edge);
}

void main() {
    vec2 p = imagePoint(gl_FragCoord.xy - uMargin);

    vec2 d = flowDir(p);
    vec2 aligned = vec2(dot(p, d) / 2.5, dot(p, vec2(-d.y, d.x)));
    vec2 warpAt = aligned / 0.45;
    vec2 warp = vec2(fbm(warpAt + uNoise.xy), fbm(warpAt + uNoise.zw)) - 0.5;
    vec2 q = p + warp * 0.4;

    float broad = fbm(aligned * 1.1 + uNoise.yz);
    float shadow = fbm(aligned * 0.8 + uNoise.wx);
    vec3 color = mix(uColors[0], uColors[2], smoothstep(0.3, 0.75, broad));
    color = mix(color, uColors[1], smoothstep(0.55, 0.85, shadow) * 0.9);
    for (int i = 0; i < uPatchCount; i++) {
        vec4 blob = uPatches[i];
        vec2 local = q - blob.xy;
        vec2 frame = vec2(dot(local, d) / 2.2, dot(local, vec2(-d.y, d.x)));
        float cover = 1.0 - smoothstep(blob.z * mix(0.4, 0.7, uPatchiness), blob.z, length(frame));
        color = mix(color, uColors[int(blob.w)], cover);
    }

    float streaks = 0.6 * vnoise(vec2(aligned.x * 3.0, aligned.y * 28.0) + uNoise.xw) + 0.4 * vnoise(vec2(aligned.x * 5.0, aligned.y * 70.0) + uNoise.zy);
    color *= mix(1.0, 0.6 + 0.8 * streaks, uPatchiness * 0.7);

    color = mix(color, uLightColor, mass(p, uLightMass, 4.0, uNoise.yx));
    color = mix(color, uAccentColor, mass(p, uAccentMass, 5.0, uNoise.wz) * step(0.001, uAccentAmount));

    outColor = vec4(color, 1.0);
}`;

const SMEAR = `${COMMON}
uniform sampler2D uScene;
uniform vec2 uMargin;
uniform float uHalfLength;
uniform float uParallax;
uniform int uSamples;
uniform float uExposure;
uniform float uValueLimit;
uniform float uGrainAmount;
uniform float uGrainCell;

out vec4 outColor;

vec2 advance(vec2 p, float s) {
    if (uMode == 1) {
        vec2 v = p - uCenter;
        float angle = s / length(v);
        float c = cos(angle);
        float n = sin(angle);
        return uCenter + vec2(v.x * c - v.y * n, v.x * n + v.y * c);
    }
    return p + uDir * s;
}

vec3 sampleScene(vec2 p) {
    vec2 px = vec2(p.x * uMinSide + 0.5 * uImage.x, 0.5 * uImage.y - p.y * uMinSide);
    return texture(uScene, (px + uMargin) / vec2(textureSize(uScene, 0))).rgb;
}

vec3 shoulder(vec3 x) {
    return mix(x, 0.8 + 0.2 * (1.0 - exp(-(x - 0.8) / 0.2)), step(0.8, x));
}

vec3 toSrgb(vec3 x) {
    return mix(x * 12.92, 1.055 * pow(x, vec3(1.0 / 2.4)) - 0.055, step(0.0031308, x));
}

vec3 curve(vec3 x) {
    vec3 low = 0.5 * pow(2.0 * x, vec3(1.1));
    vec3 high = 1.0 - 0.5 * pow(2.0 * (1.0 - x), vec3(1.1));
    return mix(low, high, step(0.5, x));
}

void main() {
    vec2 p = imagePoint(gl_FragCoord.xy);

    float across = uMode == 1 ? length(p - uCenter) : dot(p, vec2(-uDir.y, uDir.x));
    float streak = 0.65 * vnoise(vec2(across * 9.0 + uNoise.x, 0.5)) + 0.35 * vnoise(vec2(across * 31.0 + uNoise.y, 0.5));
    float halfLength = uHalfLength * (1.0 + 0.2 * (2.0 * streak - 1.0));
    if (uMode == 2) {
        float fromTop = 1.0 - gl_FragCoord.y / uImage.y;
        halfLength *= 1.0 - uParallax * (1.0 - fromTop);
    }

    float stepLength = halfLength / float(uSamples);
    float offset = hash(ivec2(gl_FragCoord.xy) + ivec2(uNoise.wz)) - 0.5;
    vec3 sum = vec3(0.0);
    float weights = 0.0;
    for (int i = -uSamples; i <= uSamples; i++) {
        float s = (float(i) + offset) * stepLength;
        float weight = 0.5 + 0.5 * cos(3.14159265 * s / (halfLength + stepLength));
        sum += sampleScene(advance(p, s)) * weight;
        weights += weight;
    }

    vec3 color = toSrgb(clamp(shoulder(sum / weights * exp2(uExposure)), 0.0, 1.0));
    color = curve(color);
    float luma = dot(color, vec3(0.2126, 0.7152, 0.0722));
    color = clamp(mix(vec3(luma), color, 1.1), 0.0, 1.0) * uValueLimit;

    vec2 grainAt = gl_FragCoord.xy / uGrainCell + uNoise.xy * 7.0;
    float grain = vnoise(grainAt) + vnoise(grainAt * 1.7 + 19.0) - 1.0;
    float tone = dot(color, vec3(0.2126, 0.7152, 0.0722));
    color += grain * uGrainAmount * (0.35 + 2.6 * tone * (1.0 - tone));

    outColor = vec4(clamp(color, 0.0, 1.0), 1.0);
}`;

const STRIP_ROWS = 128;

function compile(gl: WebGL2RenderingContext, fragment: string): WebGLProgram {
    const program = gl.createProgram();
    for (const [type, source] of [
        [gl.VERTEX_SHADER, VERTEX],
        [gl.FRAGMENT_SHADER, fragment],
    ] as const) {
        const shader = gl.createShader(type)!;
        gl.shaderSource(shader, source);
        gl.compileShader(shader);
        if (!gl.getShaderParameter(shader, gl.COMPILE_STATUS)) {
            throw new Error(gl.getShaderInfoLog(shader) ?? 'Shader failed.');
        }
        gl.attachShader(program, shader);
    }
    gl.linkProgram(program);
    if (!gl.getProgramParameter(program, gl.LINK_STATUS)) {
        throw new Error(gl.getProgramInfoLog(program) ?? 'Link failed.');
    }
    return program;
}

type Uniforms = Record<string, number | number[] | Float32Array>;

function setUniforms(
    gl: WebGL2RenderingContext,
    program: WebGLProgram,
    uniforms: Uniforms,
    ints: Set<string>,
): void {
    for (const [name, value] of Object.entries(uniforms)) {
        const location = gl.getUniformLocation(program, name);
        if (!location) continue;
        if (ints.has(name)) {
            gl.uniform1i(location, value as number);
        } else if (typeof value === 'number') {
            gl.uniform1f(location, value);
        } else if (name === 'uPatches') {
            gl.uniform4fv(location, value);
        } else if (name === 'uColors') {
            gl.uniform3fv(location, value);
        } else {
            const vector = value as number[];
            if (vector.length === 2) gl.uniform2fv(location, vector);
            else if (vector.length === 3) gl.uniform3fv(location, vector);
            else gl.uniform4fv(location, vector);
        }
    }
}

const INT_UNIFORMS = new Set([
    'uPatchCount',
    'uMode',
    'uSamples',
    'uScene',
]);

export type SmearRenderer = {
    canvas: HTMLCanvasElement;
    render(config: SmearConfig, width: number, height: number): void;
};

// The pixels depend only on the config and the requested size: all randomness comes from planScene's seeded PRNG and the shaders' integer hashes.
export function createSmearRenderer(canvas: HTMLCanvasElement): SmearRenderer {
    const context = canvas.getContext('webgl2', {
        alpha: false,
        antialias: false,
        depth: false,
        preserveDrawingBuffer: true,
    });
    if (!context) throw new Error('WebGL2 is unavailable in this browser.');
    const gl = context;

    const sceneProgram = compile(gl, SCENE);
    const smearProgram = compile(gl, SMEAR);
    const maxTexture = gl.getParameter(gl.MAX_TEXTURE_SIZE) as number;
    const framebuffer = gl.createFramebuffer();
    const sceneTexture = gl.createTexture();
    let sceneSize = [0, 0];

    function render(config: SmearConfig, width: number, height: number) {
        if (canvas.width !== width) canvas.width = width;
        if (canvas.height !== height) canvas.height = height;

        const plan = planScene(config);
        const minSide = Math.min(width, height);
        const { flow, scene, color, backdrop, finish } = config;
        const palette = generatePalette(config.seed, color);
        const aspect = [width / minSide, height / minSide];
        const toImage = (x: number, y: number) => [
            (x - 0.5) * aspect[0]!,
            (y - 0.5) * aspect[1]!,
        ];

        const angle = (flow.angle * Math.PI) / 180;
        const dir = [Math.cos(angle), -Math.sin(angle)];
        const halfDiagonal = 0.5 * Math.hypot(aspect[0]!, aspect[1]!);
        const radius = flow.arcRadius * halfDiagonal;
        const center = [dir[1]! * radius, -dir[0]! * radius];

        const halfLength = flow.smearLength / 2;
        const reachPx = halfLength * 1.2 * minSide;
        const marginPx = Math.min(
            Math.ceil(reachPx + 4),
            Math.floor((maxTexture - Math.max(width, height)) / 2),
        );
        const samples = Math.min(320, Math.max(4, Math.ceil(reachPx / 1.25)));

        const shared: Uniforms = {
            uImage: [width, height],
            uMinSide: minSide,
            uMode: MODES.indexOf(config.mode),
            uDir: dir,
            uCenter: center,
            uNoise: plan.noiseOffsets,
            uMargin: [marginPx, marginPx],
        };

        const textureWidth = width + 2 * marginPx;
        const textureHeight = height + 2 * marginPx;
        gl.bindTexture(gl.TEXTURE_2D, sceneTexture);
        if (sceneSize[0] !== textureWidth || sceneSize[1] !== textureHeight) {
            gl.texImage2D(
                gl.TEXTURE_2D,
                0,
                gl.SRGB8_ALPHA8,
                textureWidth,
                textureHeight,
                0,
                gl.RGBA,
                gl.UNSIGNED_BYTE,
                null,
            );
            gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR);
            gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
            gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
            gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
            sceneSize = [textureWidth, textureHeight];
        }

        const colors = [palette.ground, ...palette.field];
        const patches = new Float32Array(MAX_PATCHES * 4);
        plan.patches.forEach((patch, index) => {
            patches.set(
                [...toImage(patch.x, patch.y), patch.radius, patch.color],
                index * 4,
            );
        });
        const accentSize = 0.04 + 0.12 * color.accentAmount;

        gl.bindFramebuffer(gl.FRAMEBUFFER, framebuffer);
        gl.framebufferTexture2D(
            gl.FRAMEBUFFER,
            gl.COLOR_ATTACHMENT0,
            gl.TEXTURE_2D,
            sceneTexture,
            0,
        );
        gl.viewport(0, 0, textureWidth, textureHeight);
        gl.useProgram(sceneProgram);
        setUniforms(
            gl,
            sceneProgram,
            {
                ...shared,
                uPatches: patches,
                uPatchCount: plan.patches.length,
                uPatchiness: scene.patchiness,
                uColors: new Float32Array(colors.flat()),
                uLightColor: palette.light,
                uAccentColor: palette.accent,
                uLightMass: [
                    ...toImage(scene.lightX, scene.lightY),
                    scene.lightSize,
                ],
                uAccentMass: [
                    ...toImage(plan.accent.x, plan.accent.y),
                    accentSize,
                ],
                uAccentAmount: color.accentAmount,
            },
            INT_UNIFORMS,
        );
        gl.drawArrays(gl.TRIANGLES, 0, 3);

        gl.bindFramebuffer(gl.FRAMEBUFFER, null);
        gl.viewport(0, 0, width, height);
        gl.useProgram(smearProgram);
        gl.activeTexture(gl.TEXTURE0);
        gl.bindTexture(gl.TEXTURE_2D, sceneTexture);
        setUniforms(
            gl,
            smearProgram,
            {
                ...shared,
                uScene: 0,
                uHalfLength: halfLength,
                uParallax: flow.parallaxFalloff,
                uSamples: samples,
                uExposure: color.exposure,
                uValueLimit: backdrop.valueLimit,
                uGrainAmount: finish.grainAmount,
                uGrainCell: 1.2 * (minSide / 1080),
            },
            INT_UNIFORMS,
        );
        // Strips keep each GPU submission short, so large exports do not trip the driver watchdog.
        gl.enable(gl.SCISSOR_TEST);
        for (let row = 0; row < height; row += STRIP_ROWS) {
            gl.scissor(0, row, width, STRIP_ROWS);
            gl.drawArrays(gl.TRIANGLES, 0, 3);
            gl.flush();
        }
        gl.disable(gl.SCISSOR_TEST);
    }

    return { canvas, render };
}
