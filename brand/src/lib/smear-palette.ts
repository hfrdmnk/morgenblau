import { mulberry32 } from './smear-config';

export type Rgb = [number, number, number];

export type Palette = {
    ground: Rgb;
    field: [Rgb, Rgb, Rgb];
    light: Rgb;
    accent: Rgb;
};

export type PaletteKnobs = { hue: number; hueSpread: number; chroma: number };

function oklchToLinear(l: number, c: number, hueDegrees: number): Rgb {
    const h = (hueDegrees * Math.PI) / 180;
    const a = c * Math.cos(h);
    const b = c * Math.sin(h);
    const lp = (l + 0.3963377774 * a + 0.2158037573 * b) ** 3;
    const mp = (l - 0.1055613458 * a - 0.0638541728 * b) ** 3;
    const sp = (l - 0.0894841775 * a - 1.291485548 * b) ** 3;
    return [
        4.0767416621 * lp - 3.3077115913 * mp + 0.2309699292 * sp,
        -1.2684380046 * lp + 2.6097574011 * mp - 0.3413193965 * sp,
        -0.0041960863 * lp - 0.7034186147 * mp + 1.707614701 * sp,
    ];
}

// Light tones lean towards warm yellow and dark tones towards blue, as daylight does; this keeps palettes from reading as flat tints.
function shiftHue(l: number, hue: number): number {
    const target = l > 0.55 ? 90 : 265;
    const delta = ((target - hue + 540) % 360) - 180;
    return hue + delta * 0.35 * Math.min(1, Math.abs(l - 0.55) * 2.5);
}

// Keeps lightness and hue and gives up chroma until the colour fits sRGB.
function color(l: number, c: number, baseHue: number): Rgb {
    const hue = shiftHue(l, baseHue);
    let low = 0;
    let high = c;
    for (let i = 0; i < 16; i += 1) {
        const mid = (low + high) / 2;
        const rgb = oklchToLinear(l, mid, hue);
        if (rgb.every((channel) => channel >= 0 && channel <= 1)) low = mid;
        else high = mid;
    }
    return oklchToLinear(l, low, hue).map((channel) =>
        Math.min(1, Math.max(0, channel)),
    ) as Rgb;
}

// Analogous colours spread around one hue, a near-white light, and a complementary accent. The seed varies them inside the knobs.
export function generatePalette(seed: number, knobs: PaletteKnobs): Palette {
    const random = mulberry32(seed ^ 0x5eed1e55);
    const { hueSpread, chroma } = knobs;
    const hue = knobs.hue + (random() - 0.5) * 12;
    const near = () => hue + hueSpread * (random() - 0.5);
    return {
        ground: color(0.58 + random() * 0.2, chroma * 0.8, hue + hueSpread * (random() - 0.5) * 0.4),
        field: [
            color(0.26 + random() * 0.12, chroma * 0.6, near()),
            color(0.5 + random() * 0.15, chroma * (0.8 + random() * 0.3), near()),
            color(0.75 + random() * 0.12, chroma * (0.7 + random() * 0.4), near()),
        ],
        light: color(0.95 + random() * 0.03, 0.012 + random() * 0.02, near()),
        accent: color(
            0.7 + random() * 0.12,
            Math.max(chroma, 0.1),
            hue + 180 + (random() - 0.5) * 60,
        ),
    };
}
