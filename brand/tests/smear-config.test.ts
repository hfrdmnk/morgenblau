import { describe, expect, test } from 'bun:test';

import {
    BOUNDS,
    DEFAULT_CONFIG,
    configForSeed,
    normalizeConfig,
    planScene,
    serializeConfig,
} from '../src/lib/smear-config';

describe('normalizeConfig', () => {
    test('round-trips a saved config unchanged', () => {
        const config = { ...structuredClone(DEFAULT_CONFIG), seed: 2794913770 };
        config.flow.angle = -37;
        config.scene.lightSize = 0.31;
        expect(normalizeConfig(JSON.parse(serializeConfig(config)))).toEqual(
            config,
        );
    });

    test('clamps out-of-range and missing values into the knob bounds', () => {
        const config = normalizeConfig({
            seed: -1,
            mode: 'spiral',
            size: { width: 99999, height: 10 },
            flow: { smearLength: 50, angle: Number.NaN },
            scene: { lightSize: -3 },
            color: { hue: 999, chroma: "x" },
        });
        expect(config.seed).toBe(4294967295);
        expect(config.mode).toBe(DEFAULT_CONFIG.mode);
        expect(config.size).toEqual({ width: 4096, height: 64 });
        expect(config.flow.smearLength).toBe(BOUNDS.flow.smearLength.max);
        expect(config.flow.angle).toBe(BOUNDS.flow.angle.default);
        expect(config.scene.lightSize).toBe(BOUNDS.scene.lightSize.min);
        expect(config.color.hue).toBe(BOUNDS.color.hue.max);
        expect(config.color.chroma).toBe(BOUNDS.color.chroma.default);
    });
});

test('a seed always gives the same starting config, and keeps size and backdrop', () => {
    const current = structuredClone(DEFAULT_CONFIG);
    current.size = { width: 1080, height: 1350 };
    current.backdrop.valueLimit = 0.5;
    const first = configForSeed(current, 7);
    expect(configForSeed(structuredClone(DEFAULT_CONFIG), 7).flow).toEqual(first.flow);
    expect(configForSeed(current, 8).flow).not.toEqual(first.flow);
    expect(first.seed).toBe(7);
    expect(first.size).toEqual(current.size);
    expect(first.backdrop.valueLimit).toBe(0.5);
});

test('planScene depends only on the config', () => {
    const config = { ...structuredClone(DEFAULT_CONFIG), seed: 42 };
    expect(planScene(config)).toEqual(planScene(structuredClone(config)));
    expect(planScene({ ...config, seed: 43 })).not.toEqual(planScene(config));
});
