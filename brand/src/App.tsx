import { DialRoot, useDialKitController } from 'dialkit';
import 'dialkit/styles.css';
import { DiceFive, Download, FloppyDisk } from 'iconoir-react';
import { useEffect, useMemo, useRef, useState } from 'react';

import {
    BOUNDS,
    DEFAULT_CONFIG,
    MODES,
    type Mode,
    SIZE_BOUNDS,
    SIZE_PRESETS,
    SMEAR_CONFIG_VERSION,
    type SmearConfig,
    normalizeConfig,
    configForSeed,
    randomSeed,
    serializeConfig,
} from './lib/smear-config';
import { type SmearRenderer, createSmearRenderer } from './lib/smear-renderer';

const STORAGE_KEY = 'abstract-studio:smear:v2';
const CUSTOM_SIZE = 'Custom';
const THUMBNAIL_WIDTH = 240;
const EXPORT_SCALES = [1, 2, 3];
const DRAFT_WINDOW_MS = 300;

const SAVED_KEY = 'abstract-studio:saved:v2';

type Saved = { savedAt: string; config: SmearConfig };

function loadSaved(): Saved[] {
    const stored = localStorage.getItem(SAVED_KEY);
    if (!stored) return [];
    return (JSON.parse(stored) as Saved[]).map((saved) => ({
        savedAt: saved.savedAt,
        config: normalizeConfig(saved.config),
    }));
}

type Knob = [number, number, number, number];

function knobs<T extends Record<string, { min: number; max: number; step: number }>>(
    group: T,
    values: { [K in keyof T]: number },
): { [K in keyof T]: Knob } {
    return Object.fromEntries(
        Object.entries(group).map(([key, bound]) => [
            key,
            [values[key]!, bound.min, bound.max, bound.step],
        ]),
    ) as { [K in keyof T]: Knob };
}

function createDialConfig(config: SmearConfig) {
    return {
        flow: knobs(BOUNDS.flow, config.flow),
        scene: knobs(BOUNDS.scene, config.scene),
        color: knobs(BOUNDS.color, config.color),
        backdrop: knobs(BOUNDS.backdrop, config.backdrop),
        finish: knobs(BOUNDS.finish, config.finish),
    };
}

function loadInitialConfig(): SmearConfig {
    const saved = localStorage.getItem(STORAGE_KEY);
    if (saved) return normalizeConfig(JSON.parse(saved));
    return configForSeed(DEFAULT_CONFIG, randomSeed());
}

function sizeLabel(size: SmearConfig['size']): string {
    const preset = Object.entries(SIZE_PRESETS).find(
        ([, value]) =>
            value.width === size.width && value.height === size.height,
    );
    return preset ? preset[0] : CUSTOM_SIZE;
}

function downloadBlob(blob: Blob, filename: string): void {
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement('a');
    anchor.href = url;
    anchor.download = filename;
    anchor.click();
    setTimeout(() => URL.revokeObjectURL(url), 0);
}

let offlineRenderer: SmearRenderer | undefined;

function renderOffline(
    config: SmearConfig,
    width: number,
    height: number,
): HTMLCanvasElement {
    offlineRenderer ??= createSmearRenderer(document.createElement('canvas'));
    offlineRenderer.render(config, width, height);
    return offlineRenderer.canvas;
}

export default function App() {
    const [initialConfig] = useState(loadInitialConfig);
    const [seed, setSeed] = useState(initialConfig.seed);
    const [mode, setMode] = useState<Mode>(initialConfig.mode);
    const [size, setSize] = useState(initialConfig.size);
    const [customSize, setCustomSize] = useState(
        sizeLabel(initialConfig.size) === CUSTOM_SIZE,
    );
    const [exportScale, setExportScale] = useState(2);
    const [status, setStatus] = useState('');
    const [error, setError] = useState('');
    const [saved, setSaved] = useState(loadSaved);
    const [thumbnails, setThumbnails] = useState<Record<string, string>>({});
    const [stageSize, setStageSize] = useState({ width: 0, height: 0 });
    const stageRef = useRef<HTMLDivElement>(null);
    const canvasRef = useRef<HTMLCanvasElement>(null);
    const rendererRef = useRef<SmearRenderer | null>(null);
    const lastChangeRef = useRef(0);

    const dialConfig = useMemo(
        () => createDialConfig(initialConfig),
        [initialConfig],
    );
    const dial = useDialKitController('Knobs', dialConfig, { id: 'smear' });
    const values = dial.values;

    const config: SmearConfig = useMemo(
        () => ({
            version: SMEAR_CONFIG_VERSION,
            seed,
            mode,
            size,
            flow: values.flow,
            scene: values.scene,
            color: values.color,
            backdrop: values.backdrop,
            finish: values.finish,
        }),
        [
            mode,
            seed,
            size,
            values.backdrop,
            values.color,
            values.finish,
            values.flow,
            values.scene,
        ],
    );

    const fit = useMemo(() => {
        const scale = Math.min(
            stageSize.width / size.width,
            stageSize.height / size.height,
        );
        const width = Math.max(1, Math.floor(size.width * scale));
        const height = Math.max(1, Math.floor(size.height * scale));
        const pixelWidth = Math.min(
            size.width,
            Math.round(width * window.devicePixelRatio),
        );
        return {
            width,
            height,
            pixelWidth,
            pixelHeight: Math.round((pixelWidth * size.height) / size.width),
        };
    }, [size, stageSize]);

    useEffect(() => {
        const stage = stageRef.current!;
        const observer = new ResizeObserver(([entry]) => {
            const { width, height } = entry!.contentRect;
            setStageSize({ width, height });
        });
        observer.observe(stage);
        return () => observer.disconnect();
    }, []);

    useEffect(() => {
        if (fit.width < 2 || fit.height < 2) return;
        try {
            rendererRef.current ??= createSmearRenderer(canvasRef.current!);
        } catch (caught) {
            setError(caught instanceof Error ? caught.message : String(caught));
            return;
        }
        const renderer = rendererRef.current;
        const now = performance.now();
        const dragging = now - lastChangeRef.current < DRAFT_WINDOW_MS;
        lastChangeRef.current = now;
        const scale = dragging ? 0.5 : 1;
        const frame = requestAnimationFrame(() => {
            renderer.render(
                config,
                Math.max(1, Math.round(fit.pixelWidth * scale)),
                Math.max(1, Math.round(fit.pixelHeight * scale)),
            );
        });
        const settle = dragging
            ? window.setTimeout(() => {
                  renderer.render(config, fit.pixelWidth, fit.pixelHeight);
              }, DRAFT_WINDOW_MS)
            : undefined;
        return () => {
            cancelAnimationFrame(frame);
            window.clearTimeout(settle);
        };
    }, [config, fit]);

    useEffect(() => {
        localStorage.setItem(STORAGE_KEY, serializeConfig(config));
    }, [config]);

    useEffect(() => {
        const missing = saved.filter((item) => !(item.savedAt in thumbnails));
        if (missing.length === 0) return;
        const rendered: Record<string, string> = {};
        for (const item of missing) {
            const { width, height } = item.config.size;
            rendered[item.savedAt] = renderOffline(
                item.config,
                THUMBNAIL_WIDTH,
                Math.round((THUMBNAIL_WIDTH * height) / width),
            ).toDataURL();
        }
        setThumbnails((current) => ({ ...current, ...rendered }));
    }, [saved, thumbnails]);

    function applyConfig(next: SmearConfig) {
        setSeed(next.seed);
        setMode(next.mode);
        setSize(next.size);
        setCustomSize(sizeLabel(next.size) === CUSTOM_SIZE);
        dial.setValues({
            flow: next.flow,
            scene: next.scene,
            color: next.color,
            backdrop: next.backdrop,
            finish: next.finish,
        });
    }

    function newSeed() {
        applyConfig(configForSeed(config, randomSeed()));
        setStatus('');
    }

    async function save() {
        const next = [
            { savedAt: new Date().toISOString(), config },
            ...saved,
        ];
        localStorage.setItem(SAVED_KEY, JSON.stringify(next));
        setSaved(next);
        try {
            await navigator.clipboard.writeText(serializeConfig(config));
            setStatus('Saved to this browser and copied as JSON');
        } catch {
            setStatus('Saved to this browser');
        }
    }

    const maxScale = Math.max(
        1,
        Math.floor(SIZE_BOUNDS.max / Math.max(size.width, size.height)),
    );
    const scale = Math.min(exportScale, maxScale);
    const exportWidth = size.width * scale;
    const exportHeight = size.height * scale;

    function exportPng() {
        const canvas = renderOffline(config, exportWidth, exportHeight);
        canvas.toBlob((blob) => {
            if (!blob) {
                setError('PNG export failed.');
                return;
            }
            downloadBlob(
                blob,
                `smear-${mode}-${seed}-${exportWidth}x${exportHeight}.png`,
            );
            setStatus(`Exported ${exportWidth}×${exportHeight} PNG`);
        }, 'image/png');
    }

    function updateCustomSize(axis: 'width' | 'height', value: number) {
        if (!Number.isFinite(value)) return;
        setSize((current) => ({
            ...current,
            [axis]: Math.round(
                Math.min(SIZE_BOUNDS.max, Math.max(SIZE_BOUNDS.min, value)),
            ),
        }));
    }

    return (
        <main className="studio">
            <header className="studio-toolbar">
                <div className="studio-tabs" role="tablist" aria-label="Mode">
                    {MODES.map((option) => (
                        <button
                            key={option}
                            type="button"
                            role="tab"
                            aria-selected={mode === option}
                            className="studio-tab"
                            onClick={() => setMode(option)}
                        >
                            {option[0]!.toUpperCase() + option.slice(1)}
                        </button>
                    ))}
                </div>
                <div className="studio-seed">
                    <p>
                        Seed <output>{seed}</output>
                    </p>
                    <button
                        type="button"
                        className="studio-icon-button"
                        aria-label="New seed"
                        title="New seed"
                        onClick={newSeed}
                    >
                        <DiceFive aria-hidden="true" />
                    </button>
                    <button
                        type="button"
                        className="studio-icon-button"
                        aria-label="Save config"
                        title="Save config"
                        onClick={() => void save()}
                    >
                        <FloppyDisk aria-hidden="true" />
                    </button>
                </div>
                <p className="studio-status" aria-live="polite" title={status}>
                    {status}
                </p>
                <label className="studio-size">
                    <span className="studio-visually-hidden">Export size</span>
                    <select
                        value={customSize ? CUSTOM_SIZE : sizeLabel(size)}
                        onChange={(event) => {
                            const label = event.target.value;
                            setCustomSize(label === CUSTOM_SIZE);
                            if (label !== CUSTOM_SIZE) {
                                setSize({
                                    ...SIZE_PRESETS[
                                        label as keyof typeof SIZE_PRESETS
                                    ],
                                });
                            }
                        }}
                    >
                        {[...Object.keys(SIZE_PRESETS), CUSTOM_SIZE].map(
                            (label) => (
                                <option key={label}>{label}</option>
                            ),
                        )}
                    </select>
                </label>
                {customSize ? (
                    <div className="studio-custom-size">
                        {(['width', 'height'] as const).map((axis) => (
                            <label key={`${axis}-${size[axis]}`}>
                                <span className="studio-visually-hidden">
                                    {axis}
                                </span>
                                <input
                                    type="number"
                                    min={SIZE_BOUNDS.min}
                                    max={SIZE_BOUNDS.max}
                                    defaultValue={size[axis]}
                                    onBlur={(event) =>
                                        updateCustomSize(
                                            axis,
                                            event.target.valueAsNumber,
                                        )
                                    }
                                    onKeyDown={(event) => {
                                        if (event.key === 'Enter') {
                                            updateCustomSize(
                                                axis,
                                                event.currentTarget
                                                    .valueAsNumber,
                                            );
                                        }
                                    }}
                                />
                            </label>
                        ))}
                    </div>
                ) : null}
                <label className="studio-size">
                    <span className="studio-visually-hidden">Export scale</span>
                    <select
                        value={scale}
                        onChange={(event) =>
                            setExportScale(Number(event.target.value))
                        }
                    >
                        {EXPORT_SCALES.map((option) => (
                            <option
                                key={option}
                                value={option}
                                disabled={option > maxScale}
                            >
                                {option}×
                            </option>
                        ))}
                    </select>
                </label>
                <button
                    type="button"
                    className="studio-button"
                    onClick={exportPng}
                >
                    <Download aria-hidden="true" />
                    Export {exportWidth}×{exportHeight}
                </button>
            </header>

            <div ref={stageRef} className="studio-stage">
                <canvas
                    ref={canvasRef}
                    className="studio-canvas"
                    style={{ width: fit.width, height: fit.height }}
                    aria-label="Generated image. Click for a new seed."
                    onClick={newSeed}
                />
            </div>

            <aside className="studio-knobs" aria-label="Knobs">
                <DialRoot mode="inline" theme="dark" productionEnabled />
            </aside>

            <nav className="studio-rail" aria-label="Saved configs">
                {saved.length === 0 ? (
                    <p className="studio-rail-empty">
                        Saved configs appear here.
                    </p>
                ) : (
                    saved.map((item) => (
                        <button
                            key={item.savedAt}
                            type="button"
                            className="studio-rail-item"
                            title={`Saved ${new Date(item.savedAt).toLocaleString()}`}
                            onClick={() => {
                                applyConfig(item.config);
                                setStatus(`Applied ${item.config.mode} · ${item.config.seed}`);
                            }}
                        >
                            {thumbnails[item.savedAt] ? (
                                <img src={thumbnails[item.savedAt]} alt="" />
                            ) : null}
                            <span>
                                {item.config.mode} · {item.config.seed}
                            </span>
                        </button>
                    ))
                )}
            </nav>

            {error ? (
                <p className="studio-error" role="alert">
                    {error}
                </p>
            ) : null}
        </main>
    );
}
