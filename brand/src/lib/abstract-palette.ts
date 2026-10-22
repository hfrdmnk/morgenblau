import type {
    PaletteAdjustments,
    PaletteColor,
} from './abstract-generator';

type Oklab = { l: number; a: number; b: number };
type WeightedColor = Oklab & { weight: number };

const clamp = (value: number, min = 0, max = 1) =>
    Math.min(max, Math.max(min, value));

function srgbToLinear(value: number): number {
    const normalized = value / 255;
    return normalized <= 0.04045
        ? normalized / 12.92
        : ((normalized + 0.055) / 1.055) ** 2.4;
}

function linearToSrgb(value: number): number {
    const normalized =
        value <= 0.0031308
            ? 12.92 * value
            : 1.055 * value ** (1 / 2.4) - 0.055;
    return Math.round(clamp(normalized) * 255);
}

export function rgbToOklab(red: number, green: number, blue: number): Oklab {
    const r = srgbToLinear(red);
    const g = srgbToLinear(green);
    const b = srgbToLinear(blue);
    const l = Math.cbrt(0.4122214708 * r + 0.5363325363 * g + 0.0514459929 * b);
    const m = Math.cbrt(0.2119034982 * r + 0.6806995451 * g + 0.1073969566 * b);
    const s = Math.cbrt(0.0883024619 * r + 0.2817188376 * g + 0.6299787005 * b);

    return {
        l: 0.2104542553 * l + 0.793617785 * m - 0.0040720468 * s,
        a: 1.9779984951 * l - 2.428592205 * m + 0.4505937099 * s,
        b: 0.0259040371 * l + 0.7827717662 * m - 0.808675766 * s,
    };
}

export function oklabToHex({ l, a, b }: Oklab): string {
    const lRoot = l + 0.3963377774 * a + 0.2158037573 * b;
    const mRoot = l - 0.1055613458 * a - 0.0638541728 * b;
    const sRoot = l - 0.0894841775 * a - 1.291485548 * b;
    const linearL = lRoot ** 3;
    const linearM = mRoot ** 3;
    const linearS = sRoot ** 3;
    const red = linearToSrgb(
        4.0767416621 * linearL -
            3.3077115913 * linearM +
            0.2309699292 * linearS,
    );
    const green = linearToSrgb(
        -1.2684380046 * linearL +
            2.6097574011 * linearM -
            0.3413193965 * linearS,
    );
    const blue = linearToSrgb(
        -0.0041960863 * linearL -
            0.7034186147 * linearM +
            1.707614701 * linearS,
    );

    return `#${[red, green, blue]
        .map((channel) => channel.toString(16).padStart(2, '0'))
        .join('')
        .toUpperCase()}`;
}

export function hexToOklab(hex: string): Oklab {
    const normalized = hex.replace('#', '');
    const red = Number.parseInt(normalized.slice(0, 2), 16);
    const green = Number.parseInt(normalized.slice(2, 4), 16);
    const blue = Number.parseInt(normalized.slice(4, 6), 16);
    return rgbToOklab(red, green, blue);
}

function colorDistance(first: Oklab, second: Oklab): number {
    return Math.hypot(
        first.l - second.l,
        first.a - second.a,
        first.b - second.b,
    );
}

function adjustColor(hex: string, adjustments: PaletteAdjustments): string {
    const color = hexToOklab(hex);
    let chroma = Math.hypot(color.a, color.b);
    let hue = Math.atan2(color.b, color.a);

    chroma *= 1 + adjustments.saturation;
    chroma *= 1 - adjustments.mutedness * 0.72;
    chroma *= 1 - adjustments.vintageFade * 0.28;
    hue += (adjustments.hueShift * Math.PI) / 180;

    let lightness = color.l + adjustments.lightness * 0.24;
    lightness = 0.5 + (lightness - 0.5) * (1 + adjustments.contrast * 0.65);
    lightness += adjustments.vintageFade * (0.09 - lightness * 0.07);
    lightness -=
        adjustments.shadowDepth *
        Math.max(0, 0.58 - lightness) *
        0.42;

    let a = Math.cos(hue) * chroma;
    let b = Math.sin(hue) * chroma;
    a += adjustments.temperature * 0.022;
    b += adjustments.temperature * 0.045;
    b += adjustments.vintageFade * 0.012;

    return oklabToHex({
        l: clamp(lightness, 0.04, 0.97),
        a,
        b,
    });
}

export function adjustPalette(
    palette: PaletteColor[],
    adjustments: PaletteAdjustments,
): string[] {
    return palette
        .map((color) =>
            color.locked ? color.hex.toUpperCase() : adjustColor(color.hex, adjustments),
        );
}

function dominantColors(imageData: ImageData, count: number): string[] {
    const bins = new Map<
        number,
        { count: number; red: number; green: number; blue: number }
    >();

    for (let index = 0; index < imageData.data.length; index += 4) {
        if (imageData.data[index + 3] < 220) continue;
        const red = imageData.data[index];
        const green = imageData.data[index + 1];
        const blue = imageData.data[index + 2];
        const key = (red >> 4) * 256 + (green >> 4) * 16 + (blue >> 4);
        const bin = bins.get(key);
        if (bin) {
            bin.count += 1;
            bin.red += red;
            bin.green += green;
            bin.blue += blue;
        } else {
            bins.set(key, { count: 1, red, green, blue });
        }
    }

    const colors: WeightedColor[] = [...bins.values()].map((bin) => {
        const color = rgbToOklab(
            bin.red / bin.count,
            bin.green / bin.count,
            bin.blue / bin.count,
        );
        return { ...color, weight: Math.sqrt(bin.count) };
    });
    if (colors.length === 0) return [];

    const centers: Oklab[] = [];
    while (centers.length < Math.min(count, colors.length)) {
        let best = colors[0];
        let bestScore = -1;
        for (const color of colors) {
            const chroma = Math.hypot(color.a, color.b);
            const distance =
                centers.length === 0
                    ? 0.16 + chroma
                    : Math.min(
                          ...centers.map((center) =>
                              colorDistance(color, center),
                          ),
                      );
            const score = distance * (0.45 + Math.log1p(color.weight));
            if (score > bestScore) {
                best = color;
                bestScore = score;
            }
        }
        centers.push({ l: best.l, a: best.a, b: best.b });
    }

    let clusterWeights = new Array<number>(centers.length).fill(0);
    for (let iteration = 0; iteration < 7; iteration += 1) {
        const sums = centers.map(() => ({
            l: 0,
            a: 0,
            b: 0,
            weight: 0,
        }));
        for (const color of colors) {
            let cluster = 0;
            let nearest = Number.POSITIVE_INFINITY;
            centers.forEach((center, centerIndex) => {
                const distance = colorDistance(color, center);
                if (distance < nearest) {
                    nearest = distance;
                    cluster = centerIndex;
                }
            });
            sums[cluster].l += color.l * color.weight;
            sums[cluster].a += color.a * color.weight;
            sums[cluster].b += color.b * color.weight;
            sums[cluster].weight += color.weight;
        }
        clusterWeights = sums.map((sum) => sum.weight);
        centers.forEach((center, index) => {
            const sum = sums[index];
            if (sum.weight === 0) return;
            center.l = sum.l / sum.weight;
            center.a = sum.a / sum.weight;
            center.b = sum.b / sum.weight;
        });
    }

    return centers
        .map((color, index) => ({ color, weight: clusterWeights[index] }))
        .sort((first, second) => second.weight - first.weight)
        .reduce<Oklab[]>((distinct, candidate) => {
            if (
                distinct.every(
                    (color) => colorDistance(color, candidate.color) >= 0.045,
                )
            ) {
                distinct.push(candidate.color);
            }
            return distinct;
        }, [])
        .slice(0, count)
        .map(oklabToHex);
}

export async function extractPaletteFromImage(
    file: File,
    count = 8,
): Promise<string[]> {
    const bitmap = await createImageBitmap(file);
    const longestSide = 72;
    const scale = Math.min(
        1,
        longestSide / Math.max(bitmap.width, bitmap.height),
    );
    const width = Math.max(1, Math.round(bitmap.width * scale));
    const height = Math.max(1, Math.round(bitmap.height * scale));
    const canvas = document.createElement('canvas');
    canvas.width = width;
    canvas.height = height;
    const context = canvas.getContext('2d', {
        alpha: false,
        willReadFrequently: true,
    });
    if (!context) {
        bitmap.close();
        throw new Error('Canvas color sampling is not available in this browser.');
    }

    context.drawImage(bitmap, 0, 0, width, height);
    bitmap.close();
    return dominantColors(context.getImageData(0, 0, width, height), count);
}
