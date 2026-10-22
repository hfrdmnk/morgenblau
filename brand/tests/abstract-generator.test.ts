import { describe, expect, test } from 'bun:test';

import {
    DEFAULT_RECIPE,
    generateAbstractSvg,
    parseRecipe,
} from '../src/lib/abstract-generator';

describe('parseRecipe', () => {
    test('merges known imported properties into the current recipe', () => {
        const current = structuredClone(DEFAULT_RECIPE);
        current.seed = 'current-seed';
        current.settings.direction = 73;
        current.settings.grain = 0.18;

        const imported = parseRecipe(
            JSON.stringify({
                seed: 'imported-seed',
                settings: {
                    direction: -20,
                    turbulence: 0.9,
                    displacement: 50,
                    colorMixing: 1,
                    removedSetting: 42,
                },
                removedRootProperty: true,
            }),
            current,
        );

        expect(imported.seed).toBe('imported-seed');
        expect(imported.settings.direction).toBe(-20);
        expect(imported.settings.grain).toBe(0.18);
        expect(imported).not.toHaveProperty('removedRootProperty');
        expect(imported.settings).not.toHaveProperty('removedSetting');
        expect(imported.settings).not.toHaveProperty('turbulence');
        expect(imported.settings).not.toHaveProperty('displacement');
        expect(imported.settings).not.toHaveProperty('colorMixing');
    });

    test('uses slider midpoints as factory defaults', () => {
        expect(DEFAULT_RECIPE.settings.direction).toBe(0);
        expect(DEFAULT_RECIPE.settings.flowScale).toBe(1.5);
        expect(DEFAULT_RECIPE.settings.bend).toBe(1);
        expect(DEFAULT_RECIPE.settings.vortex).toBe(1);
        expect(DEFAULT_RECIPE.settings.density).toBe(0.5);
        expect(DEFAULT_RECIPE.settings.pathLength).toBe(0.6);
        expect(DEFAULT_RECIPE.settings.strokeScale).toBe(1);
        expect(DEFAULT_RECIPE.settings.cropX).toBe(0);
        expect(DEFAULT_RECIPE.settings.cropY).toBe(0);
        expect(DEFAULT_RECIPE.paletteAdjustments.saturation).toBe(0);
        expect(DEFAULT_RECIPE.paletteAdjustments.hueShift).toBe(0);
        expect(DEFAULT_RECIPE.paletteAdjustments.lightness).toBe(0);
        expect(DEFAULT_RECIPE.paletteAdjustments.contrast).toBe(0);
        expect(DEFAULT_RECIPE.paletteAdjustments.temperature).toBe(0);
        expect(DEFAULT_RECIPE.paletteAdjustments.mutedness).toBe(0.5);
        expect(DEFAULT_RECIPE.paletteAdjustments.vintageFade).toBe(0.5);
        expect(DEFAULT_RECIPE.paletteAdjustments.shadowDepth).toBe(0.5);
        expect(DEFAULT_RECIPE.settings.softness).toBe(75);
        expect(DEFAULT_RECIPE.settings.motionBlur).toBe(50);
        expect(DEFAULT_RECIPE.settings.grain).toBe(1);
        expect(DEFAULT_RECIPE.settings.shadowIntensity).toBe(0.5);
    });

    test('rejects an invalid value for a known property', () => {
        expect(() =>
            parseRecipe(
                JSON.stringify({
                    settings: {
                        direction: 'left',
                    },
                }),
                DEFAULT_RECIPE,
            ),
        ).toThrow('Recipe has invalid generator settings.');
    });

    test('renders one stronger grain filter without displacement filters', () => {
        const svg = generateAbstractSvg(DEFAULT_RECIPE, 0);

        expect(svg).not.toContain('<feDisplacementMap');
        expect(svg.match(/<feTurbulence/g)).toHaveLength(1);
        expect(svg).toContain('tableValues="0 1"');
        expect(svg).toContain('mix-blend-mode:overlay');
        expect(svg).toContain('opacity="0.5"');
    });

    test('exports fluid dimensions and one clipped canvas', () => {
        const svg = generateAbstractSvg(DEFAULT_RECIPE, 0);

        expect(svg).toStartWith(
            '<svg xmlns="http://www.w3.org/2000/svg" width="100%" height="100%"',
        );
        expect(svg).toContain('viewBox="0 0 1600 900"');
        expect(svg).toContain('preserveAspectRatio="none"');
        expect(svg).toContain('overflow="hidden"');
        expect(svg).toContain(
            'clipPathUnits="userSpaceOnUse"><rect width="1600" height="900"',
        );
        expect(svg).toMatch(
            /<g clip-path="url\(#abstract-[^)]+-canvas\)">/,
        );
        expect(svg.match(/filterUnits="objectBoundingBox"/g)).toHaveLength(
            4,
        );
        expect(svg.match(/primitiveUnits="objectBoundingBox"/g)).toHaveLength(
            4,
        );
        expect(svg).not.toContain('filterUnits="userSpaceOnUse"');
    });

    test('allows a zero stroke scale to hide flow strokes', () => {
        const recipe = structuredClone(DEFAULT_RECIPE);
        recipe.settings.strokeScale = 0;

        expect(generateAbstractSvg(recipe, 0)).toContain(
            'stroke-width="0"',
        );
    });

    test('uses the full grain range', () => {
        const recipe = structuredClone(DEFAULT_RECIPE);
        recipe.settings.grain = 2;
        const svg = generateAbstractSvg(recipe, 0);

        expect(svg).toContain('baseFrequency="640 360"');
        expect(svg).toContain('opacity="1"');
    });
});
