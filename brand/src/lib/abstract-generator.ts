import {
    adjustPalette,
    hexToOklab,
} from './abstract-palette';

export const ABSTRACT_SCHEMA_VERSION = 2;
export const ABSTRACT_GENERATOR_VERSION = '2.0.0';

export type PaletteColor = {
    id: string;
    hex: string;
    locked: boolean;
};

export type PaletteAdjustments = {
    saturation: number;
    hueShift: number;
    lightness: number;
    contrast: number;
    temperature: number;
    mutedness: number;
    vintageFade: number;
    shadowDepth: number;
};

export type GeneratorSettings = {
    direction: number;
    flowScale: number;
    bend: number;
    vortex: number;
    density: number;
    pathLength: number;
    strokeScale: number;
    softness: number;
    motionBlur: number;
    grain: number;
    shadowIntensity: number;
    cropX: number;
    cropY: number;
};

export type AspectRatio = {
    preset: '16:9' | '3:2' | '4:3' | '1:1';
    width: number;
    height: number;
};

export type AbstractRecipe = {
    schemaVersion: number;
    generatorVersion: string;
    seed: string;
    variationOffset: number;
    aspectRatio: AspectRatio;
    palette: PaletteColor[];
    paletteAdjustments: PaletteAdjustments;
    settings: GeneratorSettings;
};

export const DEFAULT_RECIPE: AbstractRecipe = {
    schemaVersion: ABSTRACT_SCHEMA_VERSION,
    generatorVersion: ABSTRACT_GENERATOR_VERSION,
    seed: 'morning-light',
    variationOffset: 0,
    aspectRatio: { preset: '16:9', width: 1600, height: 900 },
    palette: [
        { id: 'mist', hex: '#D7D6C5', locked: false },
        { id: 'fern', hex: '#637A5B', locked: false },
        { id: 'moss', hex: '#344C3B', locked: false },
        { id: 'water', hex: '#5F8393', locked: false },
        { id: 'earth', hex: '#7C5946', locked: false },
        { id: 'shade', hex: '#172B2A', locked: false },
    ],
    paletteAdjustments: {
        saturation: 0,
        hueShift: 0,
        lightness: 0,
        contrast: 0,
        temperature: 0,
        mutedness: 0.5,
        vintageFade: 0.5,
        shadowDepth: 0.5,
    },
    settings: {
        direction: 0,
        flowScale: 1.5,
        bend: 1,
        vortex: 1,
        density: 0.5,
        pathLength: 0.6,
        strokeScale: 1,
        softness: 75,
        motionBlur: 50,
        grain: 1,
        shadowIntensity: 0.5,
        cropX: 0,
        cropY: 0,
    },
};

type Point = { x: number; y: number };
type FlowContext = {
    baseAngle: number;
    flowScale: number;
    bend: number;
    vortex: number;
    vortexCenter: Point;
    vortexDirection: number;
    noiseSeed: number;
    phaseX: number;
    phaseY: number;
};

const clamp = (value: number, min: number, max: number) =>
    Math.min(max, Math.max(min, value));

function hashString(value: string): number {
    let hash = 2166136261;
    for (let index = 0; index < value.length; index += 1) {
        hash ^= value.charCodeAt(index);
        hash = Math.imul(hash, 16777619);
    }
    return hash >>> 0;
}

function createPrng(seed: string): () => number {
    let state = hashString(seed);
    return () => {
        state += 0x6d2b79f5;
        let value = state;
        value = Math.imul(value ^ (value >>> 15), value | 1);
        value ^= value + Math.imul(value ^ (value >>> 7), value | 61);
        return ((value ^ (value >>> 14)) >>> 0) / 4294967296;
    };
}

function randomBetween(random: () => number, min: number, max: number): number {
    return min + (max - min) * random();
}

function number(value: number): string {
    return Number(value.toFixed(3)).toString();
}

function escapeXml(value: string): string {
    return value
        .replaceAll('&', '&amp;')
        .replaceAll('<', '&lt;')
        .replaceAll('>', '&gt;');
}

function latticeNoise(x: number, y: number, seed: number): number {
    let value = seed;
    value ^= Math.imul(x, 374761393);
    value ^= Math.imul(y, 668265263);
    value = Math.imul(value ^ (value >>> 13), 1274126177);
    return ((value ^ (value >>> 16)) >>> 0) / 4294967295;
}

function smooth(value: number): number {
    return value * value * (3 - 2 * value);
}

function valueNoise(x: number, y: number, seed: number): number {
    const x0 = Math.floor(x);
    const y0 = Math.floor(y);
    const xFraction = smooth(x - x0);
    const yFraction = smooth(y - y0);
    const top =
        latticeNoise(x0, y0, seed) * (1 - xFraction) +
        latticeNoise(x0 + 1, y0, seed) * xFraction;
    const bottom =
        latticeNoise(x0, y0 + 1, seed) * (1 - xFraction) +
        latticeNoise(x0 + 1, y0 + 1, seed) * xFraction;
    return top * (1 - yFraction) + bottom * yFraction;
}

function fractalNoise(x: number, y: number, seed: number): number {
    let value = 0;
    let amplitude = 0.58;
    let frequency = 1;
    let normalization = 0;
    for (let octave = 0; octave < 3; octave += 1) {
        value +=
            valueNoise(x * frequency, y * frequency, seed + octave * 1013) *
            amplitude;
        normalization += amplitude;
        frequency *= 2.03;
        amplitude *= 0.46;
    }
    return value / normalization;
}

function flowVector(point: Point, context: FlowContext): Point {
    const epsilon = 0.012;
    const sample = (x: number, y: number) =>
        fractalNoise(
            (x + context.phaseX) * context.flowScale,
            (y + context.phaseY) * context.flowScale,
            context.noiseSeed,
        );
    const curlX =
        (sample(point.x, point.y + epsilon) -
            sample(point.x, point.y - epsilon)) /
        (epsilon * 2);
    const curlY =
        -(sample(point.x + epsilon, point.y) -
            sample(point.x - epsilon, point.y)) /
        (epsilon * 2);
    const curlLength = Math.hypot(curlX, curlY) || 1;
    const baseX = Math.cos(context.baseAngle);
    const baseY = Math.sin(context.baseAngle);
    const deltaX = point.x - context.vortexCenter.x;
    const deltaY = point.y - context.vortexCenter.y;
    const radius = Math.hypot(deltaX, deltaY) || 1;
    const vortexFalloff = Math.exp(-radius * radius * 2.6);
    const vortexX =
        (-deltaY / radius) *
        context.vortex *
        context.vortexDirection *
        vortexFalloff;
    const vortexY =
        (deltaX / radius) *
        context.vortex *
        context.vortexDirection *
        vortexFalloff;
    const x =
        baseX +
        (curlX / curlLength) * context.bend +
        vortexX;
    const y =
        baseY +
        (curlY / curlLength) * context.bend +
        vortexY;
    const length = Math.hypot(x, y) || 1;
    return { x: x / length, y: y / length };
}

function traceDirection(
    origin: Point,
    context: FlowContext,
    stepSize: number,
    steps: number,
    direction: -1 | 1,
): Point[] {
    const points: Point[] = [origin];
    let point = origin;
    for (let step = 0; step < steps; step += 1) {
        const vector = flowVector(point, context);
        const midpoint = {
            x: point.x + vector.x * stepSize * direction * 0.5,
            y: point.y + vector.y * stepSize * direction * 0.5,
        };
        const midpointVector = flowVector(midpoint, context);
        point = {
            x: point.x + midpointVector.x * stepSize * direction,
            y: point.y + midpointVector.y * stepSize * direction,
        };
        if (
            point.x < -0.3 ||
            point.x > 1.3 ||
            point.y < -0.45 ||
            point.y > 1.45
        ) {
            break;
        }
        points.push(point);
    }
    return points;
}

function traceStreamline(
    origin: Point,
    context: FlowContext,
    stepSize: number,
    steps: number,
): Point[] {
    const backward = traceDirection(
        origin,
        context,
        stepSize,
        steps,
        -1,
    ).reverse();
    const forward = traceDirection(origin, context, stepSize, steps, 1);
    return [...backward.slice(0, -1), ...forward];
}

function toCanvasPoint(
    point: Point,
    width: number,
    height: number,
    cropX: number,
    cropY: number,
): Point {
    return {
        x: point.x * width + cropX * width * 0.14,
        y: point.y * height + cropY * height * 0.14,
    };
}

function pointsToPath(points: Point[]): string {
    if (points.length < 2) return '';
    const segments = [`M ${number(points[0].x)} ${number(points[0].y)}`];
    for (let index = 0; index < points.length - 1; index += 1) {
        const previous = points[Math.max(0, index - 1)];
        const current = points[index];
        const next = points[index + 1];
        const following = points[Math.min(points.length - 1, index + 2)];
        const firstControl = {
            x: current.x + (next.x - previous.x) / 6,
            y: current.y + (next.y - previous.y) / 6,
        };
        const secondControl = {
            x: next.x - (following.x - current.x) / 6,
            y: next.y - (following.y - current.y) / 6,
        };
        segments.push(
            `C ${number(firstControl.x)} ${number(firstControl.y)} ${number(secondControl.x)} ${number(secondControl.y)} ${number(next.x)} ${number(next.y)}`,
        );
    }
    return segments.join(' ');
}

function paletteSubset(
    colors: string[],
    count: number,
    random: () => number,
): string[] {
    const byLightness = [...colors].sort(
        (first, second) =>
            hexToOklab(first).l - hexToOklab(second).l,
    );
    const anchors = [byLightness[0], byLightness[byLightness.length - 1]];
    const shuffled = byLightness.slice(1, -1);
    for (let index = shuffled.length - 1; index > 0; index -= 1) {
        const target = Math.floor(random() * (index + 1));
        [shuffled[index], shuffled[target]] = [
            shuffled[target],
            shuffled[index],
        ];
    }
    return [...anchors, ...shuffled.slice(0, Math.max(0, count - 2))];
}

function darkestColor(colors: string[]): string {
    return [...colors].sort(
        (first, second) =>
            hexToOklab(first).l - hexToOklab(second).l,
    )[0];
}

function lightestColor(colors: string[]): string {
    return [...colors].sort(
        (first, second) =>
            hexToOklab(second).l - hexToOklab(first).l,
    )[0];
}

function colorAt(colors: string[], index: number): string {
    return colors[index % colors.length] ?? '#172B2A';
}

function gradientDefinition(
    id: string,
    index: number,
    points: Point[],
    colors: string[],
    random: () => number,
): string {
    const first = points[0];
    const last = points[points.length - 1];
    const firstColor = colorAt(
        colors,
        Math.floor(random() * colors.length),
    );
    const secondColor =
        index % 3 === 0
            ? lightestColor(colors)
            : colorAt(colors, Math.floor(random() * colors.length));
    const thirdColor = colorAt(
        colors,
        Math.floor(random() * colors.length),
    );
    return `<linearGradient id="${id}-line-${index}" gradientUnits="userSpaceOnUse" x1="${number(first.x)}" y1="${number(first.y)}" x2="${number(last.x)}" y2="${number(last.y)}"><stop offset="0" stop-color="${firstColor}" stop-opacity="0"/><stop offset=".18" stop-color="${firstColor}" stop-opacity=".86"/><stop offset=".56" stop-color="${secondColor}" stop-opacity=".74"/><stop offset=".84" stop-color="${thirdColor}" stop-opacity=".62"/><stop offset="1" stop-color="${thirdColor}" stop-opacity="0"/></linearGradient>`;
}

function backgroundDefinitions(
    id: string,
    colors: string[],
    random: () => number,
): string {
    return `<linearGradient id="${id}-base" x1="${number(random())}" y1="${number(random() * 0.3)}" x2="${number(random())}" y2="${number(0.7 + random() * 0.3)}"><stop offset="0" stop-color="${lightestColor(colors)}" stop-opacity=".82"/><stop offset=".48" stop-color="${colorAt(colors, 2)}" stop-opacity=".72"/><stop offset="1" stop-color="${darkestColor(colors)}" stop-opacity=".58"/></linearGradient>`;
}

function filterDefinitions(
    recipe: AbstractRecipe,
    id: string,
    noiseSeed: number,
): string {
    const settings = recipe.settings;
    const { width, height } = recipe.aspectRatio;
    const grainAmount = clamp(settings.grain / 2, 0, 1);
    const grainFrequency = 0.12 + grainAmount * 0.28;
    const filterCoordinates =
        'filterUnits="objectBoundingBox" primitiveUnits="objectBoundingBox"';
    return `<filter id="${id}-soft" ${filterCoordinates} x="-.35" y="-.45" width="1.7" height="1.9"><feGaussianBlur stdDeviation="${number((settings.softness * 1.15) / width)} ${number((settings.softness * 1.15) / height)}"/></filter>
<filter id="${id}-motion" data-motion-blur="true" ${filterCoordinates} x="-.45" y="-.5" width="1.9" height="2"><feGaussianBlur stdDeviation="${number(settings.motionBlur / width)} ${number((settings.softness * 0.75) / height)}"/></filter>
<filter id="${id}-detail" ${filterCoordinates} x="-.25" y="-.3" width="1.5" height="1.6"><feGaussianBlur stdDeviation="${number((settings.softness * 0.28) / width)} ${number((settings.softness * 0.28) / height)}"/></filter>
<filter id="${id}-grain" ${filterCoordinates} x="0" y="0" width="1" height="1"><feTurbulence type="fractalNoise" baseFrequency="${number(grainFrequency * width)} ${number(grainFrequency * height)}" numOctaves="2" seed="${(noiseSeed + 59) % 997}" stitchTiles="stitch"/><feColorMatrix type="saturate" values="0"/><feComponentTransfer><feFuncA type="table" tableValues="0 1"/></feComponentTransfer></filter>`;
}

export function deriveVariationSeed(baseSeed: string, variation: number): string {
    return `${baseSeed || 'untitled'}-${hashString(`${baseSeed}::${variation}`).toString(36)}`;
}

export function generateAbstractSvg(
    recipe: AbstractRecipe,
    variation: number,
): string {
    const variationIndex = recipe.variationOffset + variation;
    const variationSeed = deriveVariationSeed(recipe.seed, variationIndex);
    const random = createPrng(
        `${recipe.generatorVersion}:flow-field:${variationSeed}`,
    );
    const id = `abstract-${hashString(variationSeed).toString(36)}`;
    const { width, height } = recipe.aspectRatio;
    const adjustedColors = adjustPalette(
        recipe.palette,
        recipe.paletteAdjustments,
    );
    const paletteCount = clamp(
        3 + Math.floor(random() * Math.max(1, adjustedColors.length - 2)),
        3,
        Math.min(7, adjustedColors.length),
    );
    const colors = paletteSubset(adjustedColors, paletteCount, random);
    const background = darkestColor(colors);
    const directionJitter = randomBetween(random, -26, 26);
    const dominantDirection = recipe.settings.direction + directionJitter;
    const baseAngle = (dominantDirection * Math.PI) / 180;
    const noiseSeed = hashString(`${variationSeed}:noise`);
    const flowContext: FlowContext = {
        baseAngle,
        flowScale:
            recipe.settings.flowScale * randomBetween(random, 0.82, 1.24),
        bend: recipe.settings.bend * randomBetween(random, 0.72, 1.36),
        vortex: recipe.settings.vortex * randomBetween(random, 0.5, 1.35),
        vortexCenter: {
            x: randomBetween(random, 0.18, 0.82),
            y: randomBetween(random, 0.12, 0.88),
        },
        vortexDirection: random() > 0.5 ? 1 : -1,
        noiseSeed,
        phaseX: randomBetween(random, -4, 4),
        phaseY: randomBetween(random, -4, 4),
    };
    const streamlineCount = Math.round(
        clamp(
            11 +
                recipe.settings.density * 16 +
                randomBetween(random, -4, 5),
            10,
            32,
        ),
    );
    const diagonal = Math.hypot(width, height);
    const baseSteps = Math.round(
        18 + recipe.settings.pathLength * 30,
    );
    const gradients: string[] = [];
    const farPaths: string[] = [];
    const motionPaths: string[] = [];
    const detailPaths: string[] = [];

    for (let index = 0; index < streamlineCount; index += 1) {
        const origin = {
            x: randomBetween(random, -0.08, 1.08),
            y: randomBetween(random, -0.12, 1.12),
        };
        const points = traceStreamline(
            origin,
            flowContext,
            randomBetween(random, 0.018, 0.033),
            Math.round(baseSteps * randomBetween(random, 0.68, 1.24)),
        ).map((point) =>
            toCanvasPoint(
                point,
                width,
                height,
                recipe.settings.cropX,
                recipe.settings.cropY,
            ),
        );
        if (points.length < 8) continue;
        gradients.push(
            gradientDefinition(id, index, points, colors, random),
        );
        const layerRoll = random();
        const isFar = layerRoll < 0.24;
        const isDetail = layerRoll > 0.78;
        const widthFactor = isFar
            ? randomBetween(random, 0.035, 0.095)
            : isDetail
              ? randomBetween(random, 0.004, 0.022)
              : randomBetween(random, 0.018, 0.065);
        const opacity = isFar
            ? randomBetween(random, 0.12, 0.28)
            : isDetail
              ? randomBetween(random, 0.12, 0.32)
              : randomBetween(random, 0.14, 0.36);
        const strokeWidth =
            diagonal *
            clamp(
                widthFactor * recipe.settings.strokeScale,
                0,
                0.115,
            );
        const path = `<path data-flow-line="1" d="${pointsToPath(points)}" fill="none" stroke="url(#${id}-line-${index})" stroke-width="${number(strokeWidth)}" stroke-linecap="round" stroke-linejoin="round" opacity="${number(opacity)}" style="mix-blend-mode:soft-light"/>`;
        if (isFar) farPaths.push(path);
        else if (isDetail) detailPaths.push(path);
        else motionPaths.push(path);
    }

    const renderedStreamlineCount =
        farPaths.length + motionPaths.length + detailPaths.length;
    const metadata = escapeXml(
        JSON.stringify({
            schemaVersion: recipe.schemaVersion,
            generatorVersion: recipe.generatorVersion,
            technique: 'curl-flow-field',
            seed: recipe.seed,
            variation: variationIndex,
            variationSeed,
            paletteColorCount: paletteCount,
            streamlineCount: renderedStreamlineCount,
        }),
    );

    const filterBounds = `<rect width="${width}" height="${height}" fill="transparent"/>`;

    return `<svg xmlns="http://www.w3.org/2000/svg" width="100%" height="100%" viewBox="0 0 ${width} ${height}" preserveAspectRatio="none" overflow="hidden" style="display:block" role="img" aria-label="Abstract flow-field composition">
<metadata>${metadata}</metadata>
<defs>
${backgroundDefinitions(id, colors, random)}
${gradients.join('')}
${filterDefinitions(recipe, id, noiseSeed)}
<clipPath id="${id}-canvas" clipPathUnits="userSpaceOnUse"><rect width="${width}" height="${height}"/></clipPath>
</defs>
<g clip-path="url(#${id}-canvas)">
<g data-technique="curl-flow-field" data-streamline-count="${renderedStreamlineCount}" data-palette-count="${paletteCount}">
<rect width="${width}" height="${height}" fill="${background}"/>
<rect width="${width}" height="${height}" fill="url(#${id}-base)" opacity=".88"/>
<g filter="url(#${id}-soft)">${filterBounds}${farPaths.join('')}</g>
<g transform="rotate(${number(dominantDirection)} ${number(width / 2)} ${number(height / 2)})" filter="url(#${id}-motion)"><g transform="rotate(${number(-dominantDirection)} ${number(width / 2)} ${number(height / 2)})">${filterBounds}${motionPaths.join('')}</g></g>
<g filter="url(#${id}-detail)">${filterBounds}${detailPaths.join('')}</g>
</g>
<rect width="${width}" height="${height}" fill="#000000" opacity="${number(recipe.settings.shadowIntensity * 0.1)}" style="mix-blend-mode:multiply"/>
<rect width="${width}" height="${height}" filter="url(#${id}-grain)" opacity="${number(clamp(recipe.settings.grain / 2, 0, 1))}" style="mix-blend-mode:overlay"/>
</g>
</svg>`;
}

export function serializeRecipe(recipe: AbstractRecipe): string {
    return JSON.stringify(recipe, null, 2);
}

function matchesShape(value: unknown, template: unknown): boolean {
    if (typeof template === 'number') {
        return typeof value === 'number' && Number.isFinite(value);
    }
    if (typeof template === 'string' || typeof template === 'boolean') {
        return typeof value === typeof template;
    }
    if (
        !value ||
        !template ||
        typeof value !== 'object' ||
        typeof template !== 'object' ||
        Array.isArray(value) ||
        Array.isArray(template)
    ) {
        return false;
    }
    return Object.entries(template).every(([key, childTemplate]) =>
        matchesShape(
            (value as Record<string, unknown>)[key],
            childTemplate,
        ),
    );
}

function numbersWithin(value: unknown, minimum: number, maximum: number): boolean {
    if (typeof value === 'number') {
        return Number.isFinite(value) && value >= minimum && value <= maximum;
    }
    if (!value || typeof value !== 'object') return true;
    return Object.values(value).every((child) =>
        numbersWithin(child, minimum, maximum),
    );
}

function isRecord(value: unknown): value is Record<string, unknown> {
    return (
        !!value &&
        typeof value === 'object' &&
        !Array.isArray(value)
    );
}

function mergeKnownProperties<T>(current: T, imported: unknown): T {
    if (!isRecord(current) || !isRecord(imported)) {
        return structuredClone(imported) as T;
    }

    const merged = structuredClone(current) as Record<string, unknown>;
    for (const [key, currentValue] of Object.entries(current)) {
        if (Object.hasOwn(imported, key)) {
            merged[key] = mergeKnownProperties(
                currentValue,
                imported[key],
            );
        }
    }
    return merged as T;
}

export function parseRecipe(
    json: string,
    current: AbstractRecipe = DEFAULT_RECIPE,
): AbstractRecipe {
    const parsed: unknown = JSON.parse(json);
    if (!isRecord(parsed)) {
        throw new Error('Recipe must be a JSON object.');
    }
    const candidate = mergeKnownProperties(current, parsed);
    if (candidate.schemaVersion !== ABSTRACT_SCHEMA_VERSION) {
        throw new Error(
            `Unsupported recipe schema ${String(candidate.schemaVersion)}.`,
        );
    }
    if (candidate.generatorVersion !== ABSTRACT_GENERATOR_VERSION) {
        throw new Error(
            `Unsupported generator version ${String(candidate.generatorVersion)}.`,
        );
    }
    if (
        typeof candidate.seed !== 'string' ||
        !candidate.aspectRatio ||
        !Array.isArray(candidate.palette) ||
        !candidate.paletteAdjustments ||
        !candidate.settings
    ) {
        throw new Error('Recipe is missing required generator data.');
    }
    if (
        typeof candidate.variationOffset !== 'number' ||
        !Number.isInteger(candidate.variationOffset) ||
        candidate.variationOffset < 0 ||
        !Number.isInteger(candidate.aspectRatio.width) ||
        !Number.isInteger(candidate.aspectRatio.height) ||
        candidate.aspectRatio.width < 64 ||
        candidate.aspectRatio.width > 8192 ||
        candidate.aspectRatio.height < 64 ||
        candidate.aspectRatio.height > 8192
    ) {
        throw new Error('Recipe has invalid dimensions or variation data.');
    }
    if (
        candidate.palette.length < 5 ||
        candidate.palette.length > 8 ||
        candidate.palette.some(
            (color) =>
                !color ||
                typeof color.id !== 'string' ||
                !/^#[0-9a-f]{6}$/i.test(color.hex) ||
                typeof color.locked !== 'boolean',
        )
    ) {
        throw new Error('Recipe has an invalid palette.');
    }
    if (
        !matchesShape(
            candidate.paletteAdjustments,
            DEFAULT_RECIPE.paletteAdjustments,
        ) ||
        !matchesShape(candidate.settings, DEFAULT_RECIPE.settings) ||
        !numbersWithin(candidate.paletteAdjustments, -200, 200) ||
        !numbersWithin(candidate.settings, -200, 200)
    ) {
        throw new Error('Recipe has invalid generator settings.');
    }
    return candidate as AbstractRecipe;
}

export function assertStandaloneSvg(svg: string): void {
    if (
        !svg.startsWith('<svg ') ||
        !/\bwidth="100%"/.test(svg) ||
        !/\bheight="100%"/.test(svg) ||
        !/\bviewBox="0 0 \d+ \d+"/.test(svg) ||
        /<image\b/i.test(svg) ||
        /(?:href|src)\s*=\s*["'](?:https?:|blob:|data:)/i.test(svg)
    ) {
        throw new Error('The generated SVG is not standalone.');
    }
}
