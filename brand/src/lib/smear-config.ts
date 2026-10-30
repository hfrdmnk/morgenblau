export const SMEAR_CONFIG_VERSION = 2;

export const MODES = ['pan', 'swing', 'window'] as const;
export type Mode = (typeof MODES)[number];

export type SmearConfig = {
    version: number;
    seed: number;
    mode: Mode;
    size: { width: number; height: number };
    flow: {
        angle: number;
        arcRadius: number;
        smearLength: number;
        parallaxFalloff: number;
    };
    scene: {
        patchiness: number;
        lightX: number;
        lightY: number;
        lightSize: number;
    };
    color: {
        hue: number;
        hueSpread: number;
        chroma: number;
        exposure: number;
        accentAmount: number;
    };
    backdrop: { valueLimit: number };
    finish: { grainAmount: number };
};

type Bound = { min: number; max: number; step: number; default: number };

// Configs are clamped into these ranges and the knobs use them. Randomize draws uniformly from them, so every value in range must give a usable image.
export const BOUNDS = {
    flow: {
        angle: { min: -180, max: 180, step: 1, default: 24 },
        arcRadius: { min: 1.5, max: 6, step: 0.05, default: 2 },
        smearLength: { min: 0.3, max: 1, step: 0.005, default: 0.6 },
        parallaxFalloff: { min: 0, max: 0.9, step: 0.01, default: 0.6 },
    },
    scene: {
        patchiness: { min: 0, max: 1, step: 0.01, default: 0.4 },
        lightX: { min: 0.1, max: 0.9, step: 0.01, default: 0.42 },
        lightY: { min: 0.1, max: 0.9, step: 0.01, default: 0.4 },
        lightSize: { min: 0.15, max: 0.45, step: 0.01, default: 0.25 },
    },
    color: {
        hue: { min: 0, max: 360, step: 1, default: 100 },
        hueSpread: { min: 20, max: 100, step: 1, default: 50 },
        chroma: { min: 0.09, max: 0.18, step: 0.005, default: 0.13 },
        exposure: { min: -0.4, max: 0.4, step: 0.01, default: 0 },
        accentAmount: { min: 0.2, max: 1, step: 0.01, default: 0.5 },
    },
    backdrop: {
        valueLimit: { min: 0.4, max: 1, step: 0.01, default: 1 },
    },
    finish: {
        grainAmount: { min: 0.02, max: 0.08, step: 0.005, default: 0.045 },
    },
} satisfies Record<string, Record<string, Bound>>;

export const SIZE_BOUNDS = { min: 64, max: 4096 };

export const SIZE_PRESETS = {
    '1920×1080': { width: 1920, height: 1080 },
    '1200×630 OG': { width: 1200, height: 630 },
    '1080×1350 portrait': { width: 1080, height: 1350 },
} as const;

export const MAX_PATCHES = 16;

function defaults<T extends Record<string, Bound>>(
    group: T,
): { [K in keyof T]: number } {
    return Object.fromEntries(
        Object.entries(group).map(([key, bound]) => [key, bound.default]),
    ) as { [K in keyof T]: number };
}

export const DEFAULT_CONFIG: SmearConfig = {
    version: SMEAR_CONFIG_VERSION,
    seed: 1,
    mode: 'swing',
    size: { ...SIZE_PRESETS['1920×1080'] },
    flow: defaults(BOUNDS.flow),
    scene: defaults(BOUNDS.scene),
    color: defaults(BOUNDS.color),
    backdrop: defaults(BOUNDS.backdrop),
    finish: defaults(BOUNDS.finish),
};

export function mulberry32(seed: number): () => number {
    let state = seed >>> 0;
    return () => {
        state = (state + 0x6d2b79f5) >>> 0;
        let t = state;
        t = Math.imul(t ^ (t >>> 15), t | 1);
        t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
        return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
    };
}

export function randomSeed(): number {
    return crypto.getRandomValues(new Uint32Array(1))[0]!;
}

function isRecord(value: unknown): value is Record<string, unknown> {
    return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function clampNumber(value: unknown, bound: Bound): number {
    if (typeof value !== 'number' || !Number.isFinite(value)) {
        return bound.default;
    }
    return Math.min(bound.max, Math.max(bound.min, value));
}

function clampGroup<T extends Record<string, Bound>>(
    value: unknown,
    group: T,
): { [K in keyof T]: number } {
    const source = isRecord(value) ? value : {};
    return Object.fromEntries(
        Object.entries(group).map(([key, bound]) => [
            key,
            clampNumber(source[key], bound),
        ]),
    ) as { [K in keyof T]: number };
}

function clampSize(value: unknown): number {
    if (typeof value !== 'number' || !Number.isFinite(value)) return 1080;
    return Math.round(
        Math.min(SIZE_BOUNDS.max, Math.max(SIZE_BOUNDS.min, value)),
    );
}

export function normalizeConfig(value: unknown): SmearConfig {
    const source = isRecord(value) ? value : {};
    const size = isRecord(source.size) ? source.size : {};
    const seed =
        typeof source.seed === 'number' && Number.isFinite(source.seed)
            ? Math.trunc(source.seed) >>> 0
            : DEFAULT_CONFIG.seed;
    return {
        version: SMEAR_CONFIG_VERSION,
        seed,
        mode: MODES.includes(source.mode as Mode)
            ? (source.mode as Mode)
            : DEFAULT_CONFIG.mode,
        size: {
            width: clampSize(size.width ?? DEFAULT_CONFIG.size.width),
            height: clampSize(size.height ?? DEFAULT_CONFIG.size.height),
        },
        flow: clampGroup(source.flow, BOUNDS.flow),
        scene: clampGroup(source.scene, BOUNDS.scene),
        color: clampGroup(source.color, BOUNDS.color),
        backdrop: clampGroup(source.backdrop, BOUNDS.backdrop),
        finish: clampGroup(source.finish, BOUNDS.finish),
    };
}

const RANDOMIZED_GROUPS = ['flow', 'scene', 'color', 'finish'] as const;

// Each seed's starting knobs. Size and the backdrop limit carry over from the current config: they describe where the image will be used, not how it looks.
export function configForSeed(config: SmearConfig, seed: number): SmearConfig {
    const random = mulberry32(seed ^ 0x9e3779b9);
    const next = structuredClone(config);
    next.seed = seed;
    next.mode = MODES[Math.floor(random() * MODES.length)]!;
    for (const group of RANDOMIZED_GROUPS) {
        const values = next[group] as Record<string, number>;
        for (const [key, bound] of Object.entries(BOUNDS[group])) {
            const value = bound.min + random() * (bound.max - bound.min);
            values[key] = Number(
                (bound.min + Math.round((value - bound.min) / bound.step) * bound.step).toFixed(4),
            );
        }
    }
    return next;
}

export function serializeConfig(config: SmearConfig): string {
    return `${JSON.stringify(config, null, 4)}\n`;
}

export type Patch = { x: number; y: number; radius: number; color: number };

export type ScenePlan = {
    patches: Patch[];
    accent: { x: number; y: number };
    noiseOffsets: [number, number, number, number];
    grainSeed: number;
};

// Positions are fractions of the image width and height, so a config keeps its composition at every output size.
export function planScene(config: SmearConfig): ScenePlan {
    const random = mulberry32(config.seed);
    const { patchiness } = config.scene;
    const count = Math.round(4 + 12 * patchiness);
    const scale = 0.55 - 0.33 * patchiness;
    const patches: Patch[] = [];
    for (let index = 0; index < count; index += 1) {
        patches.push({
            x: -0.15 + random() * 1.3,
            y: -0.15 + random() * 1.3,
            radius: scale * (0.55 + random() * 0.9),
            color: Math.floor(random() * 4),
        });
    }
    const accent = { x: 0.12 + random() * 0.76, y: 0.12 + random() * 0.76 };
    const noiseOffsets: [number, number, number, number] = [
        random() * 512,
        random() * 512,
        random() * 512,
        random() * 512,
    ];
    return {
        patches,
        accent,
        noiseOffsets,
        grainSeed: Math.floor(random() * 0xffffff),
    };
}
