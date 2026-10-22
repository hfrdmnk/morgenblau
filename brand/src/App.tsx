import { DialRoot, useDialKitController } from 'dialkit';
import 'dialkit/styles.css';
import {
    Check,
    Code,
    CurveArray,
    DiceFive,
    Download,
    SvgFormat,
    Xmark,
} from 'iconoir-react';
import {
    type ChangeEvent,
    useDeferredValue,
    useEffect,
    useMemo,
    useRef,
    useState,
} from 'react';
import { createPortal } from 'react-dom';

import {
    ABSTRACT_GENERATOR_VERSION,
    ABSTRACT_SCHEMA_VERSION,
    type AbstractRecipe,
    type AspectRatio,
    DEFAULT_RECIPE,
    type GeneratorSettings,
    type PaletteAdjustments,
    assertStandaloneSvg,
    generateAbstractSvg,
    parseRecipe,
    serializeRecipe,
} from './lib/abstract-generator';
import { extractPaletteFromImage } from './lib/abstract-palette';
import {
    randomHueShift,
    randomSeedWord,
} from './lib/random-values';

const STORAGE_KEY = 'abstract-studio:recipe:v2';
const DIAL_STORAGE_KEY = 'abstract-studio:dials:v2';

const SLIDER_RESET_VALUES: Record<
    string,
    { path: string; value: number }
> = {
    Direction: {
        path: 'flow.direction',
        value: DEFAULT_RECIPE.settings.direction,
    },
    'Flow Scale': {
        path: 'flow.flowScale',
        value: DEFAULT_RECIPE.settings.flowScale,
    },
    Bend: { path: 'flow.bend', value: DEFAULT_RECIPE.settings.bend },
    Vortex: {
        path: 'flow.vortex',
        value: DEFAULT_RECIPE.settings.vortex,
    },
    Density: {
        path: 'flow.density',
        value: DEFAULT_RECIPE.settings.density,
    },
    'Path Length': {
        path: 'flow.pathLength',
        value: DEFAULT_RECIPE.settings.pathLength,
    },
    'Stroke Scale': {
        path: 'flow.strokeScale',
        value: DEFAULT_RECIPE.settings.strokeScale,
    },
    'Crop X': {
        path: 'flow.cropX',
        value: DEFAULT_RECIPE.settings.cropX,
    },
    'Crop Y': {
        path: 'flow.cropY',
        value: DEFAULT_RECIPE.settings.cropY,
    },
    Saturation: {
        path: 'color.saturation',
        value: DEFAULT_RECIPE.paletteAdjustments.saturation,
    },
    'Hue Shift': {
        path: 'color.hueShift',
        value: DEFAULT_RECIPE.paletteAdjustments.hueShift,
    },
    Lightness: {
        path: 'color.lightness',
        value: DEFAULT_RECIPE.paletteAdjustments.lightness,
    },
    Contrast: {
        path: 'color.contrast',
        value: DEFAULT_RECIPE.paletteAdjustments.contrast,
    },
    Temperature: {
        path: 'color.temperature',
        value: DEFAULT_RECIPE.paletteAdjustments.temperature,
    },
    Mutedness: {
        path: 'color.mutedness',
        value: DEFAULT_RECIPE.paletteAdjustments.mutedness,
    },
    'Vintage Fade': {
        path: 'color.vintageFade',
        value: DEFAULT_RECIPE.paletteAdjustments.vintageFade,
    },
    'Shadow Depth': {
        path: 'color.shadowDepth',
        value: DEFAULT_RECIPE.paletteAdjustments.shadowDepth,
    },
    Softness: {
        path: 'finish.softness',
        value: DEFAULT_RECIPE.settings.softness,
    },
    'Motion Blur': {
        path: 'finish.motionBlur',
        value: DEFAULT_RECIPE.settings.motionBlur,
    },
    Grain: {
        path: 'finish.grain',
        value: DEFAULT_RECIPE.settings.grain,
    },
    'Shadow Intensity': {
        path: 'finish.shadowIntensity',
        value: DEFAULT_RECIPE.settings.shadowIntensity,
    },
};

const ASPECT_RATIOS: Record<string, AspectRatio> = {
    '16:9': { preset: '16:9', width: 1600, height: 900 },
    '3:2': { preset: '3:2', width: 1500, height: 1000 },
    '4:3': { preset: '4:3', width: 1440, height: 1080 },
    '1:1': { preset: '1:1', width: 1200, height: 1200 },
};

const slider = (
    value: number,
    min: number,
    max: number,
    step?: number,
): [number, number, number, number?] => [value, min, max, step];

function cloneRecipe(recipe: AbstractRecipe): AbstractRecipe {
    return structuredClone(recipe);
}

function createFactoryRecipe(): AbstractRecipe {
    const recipe = cloneRecipe(DEFAULT_RECIPE);
    recipe.seed = randomSeedWord();
    return recipe;
}

function loadInitialRecipe(): AbstractRecipe {
    try {
        const saved = localStorage.getItem(STORAGE_KEY);
        return saved ? parseRecipe(saved) : createFactoryRecipe();
    } catch {
        return createFactoryRecipe();
    }
}

function createDialConfig(
    recipe: AbstractRecipe,
    referenceName: string,
    isExtracting: boolean,
) {
    const { settings, paletteAdjustments } = recipe;
    return {
        seed: {
            type: 'text' as const,
            default: recipe.seed,
            placeholder: 'Seed',
        },
        aspectRatio: {
            type: 'select' as const,
            options: Object.keys(ASPECT_RATIOS),
            default: recipe.aspectRatio.preset,
        },
        uploadReference: {
            type: 'action' as const,
            label: isExtracting
                ? 'Sampling image…'
                : referenceName
                  ? `Replace ${referenceName}`
                  : 'Upload reference image',
        },
        newVariations: {
            type: 'action' as const,
            label: 'New variations',
        },
        newVariationsAndHueShift: {
            type: 'action' as const,
            label: 'New variations & hue shift',
        },
        flow: {
            direction: slider(settings.direction, -180, 180, 1),
            flowScale: slider(settings.flowScale, 0, 3, 0.01),
            bend: slider(settings.bend, 0, 2, 0.01),
            vortex: slider(settings.vortex, 0, 2, 0.01),
            density: slider(settings.density, 0, 1, 0.01),
            pathLength: slider(settings.pathLength, 0.2, 1, 0.01),
            strokeScale: slider(settings.strokeScale, 0, 2, 0.01),
            cropX: slider(settings.cropX, -1, 1, 0.01),
            cropY: slider(settings.cropY, -1, 1, 0.01),
        },
        color: {
            saturation: slider(paletteAdjustments.saturation, -1, 1, 0.01),
            hueShift: slider(paletteAdjustments.hueShift, -180, 180, 1),
            lightness: slider(paletteAdjustments.lightness, -1, 1, 0.01),
            contrast: slider(paletteAdjustments.contrast, -1, 1, 0.01),
            temperature: slider(paletteAdjustments.temperature, -1, 1, 0.01),
            mutedness: slider(paletteAdjustments.mutedness, 0, 1, 0.01),
            vintageFade: slider(paletteAdjustments.vintageFade, 0, 1, 0.01),
            shadowDepth: slider(paletteAdjustments.shadowDepth, 0, 1, 0.01),
        },
        finish: {
            softness: slider(Math.max(50, settings.softness), 50, 100, 1),
            motionBlur: slider(settings.motionBlur, 0, 100, 1),
            grain: slider(settings.grain, 0, 2, 0.01),
            shadowIntensity: slider(
                settings.shadowIntensity,
                0,
                1,
                0.01,
            ),
        },
        recipe: {
            _collapsed: true,
            importRecipe: {
                type: 'action' as const,
                label: 'Import recipe',
            },
            downloadRecipe: {
                type: 'action' as const,
                label: 'Download recipe',
            },
            reset: {
                type: 'action' as const,
                label: 'Reset parameters',
            },
        },
    };
}

function dialValuesForRecipe(recipe: AbstractRecipe) {
    return {
        seed: recipe.seed,
        aspectRatio: recipe.aspectRatio.preset,
        flow: {
            direction: recipe.settings.direction,
            flowScale: recipe.settings.flowScale,
            bend: recipe.settings.bend,
            vortex: recipe.settings.vortex,
            density: recipe.settings.density,
            pathLength: recipe.settings.pathLength,
            strokeScale: recipe.settings.strokeScale,
            cropX: recipe.settings.cropX,
            cropY: recipe.settings.cropY,
        },
        color: recipe.paletteAdjustments,
        finish: {
            softness: Math.max(50, recipe.settings.softness),
            motionBlur: recipe.settings.motionBlur,
            grain: recipe.settings.grain,
            shadowIntensity: recipe.settings.shadowIntensity,
        },
    };
}

function svgDataUrl(svg: string): string {
    return `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`;
}

function safeFilenamePart(value: string): string {
    return (
        value
            .trim()
            .toLowerCase()
            .replace(/[^a-z0-9]+/g, '-')
            .replace(/^-|-$/g, '')
            .slice(0, 48) || 'untitled'
    );
}

function downloadBlob(blob: Blob, filename: string): void {
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement('a');
    anchor.href = url;
    anchor.download = filename;
    document.body.append(anchor);
    anchor.click();
    anchor.remove();
    setTimeout(() => URL.revokeObjectURL(url), 0);
}

function verifySvg(svg: string): void {
    assertStandaloneSvg(svg);
    const document = new DOMParser().parseFromString(svg, 'image/svg+xml');
    if (document.querySelector('parsererror')) {
        throw new Error('The generated SVG could not be parsed.');
    }
}

function reactAttributeName(name: string): string {
    if (name === 'class') return 'className';
    if (name.startsWith('aria-') || name.startsWith('data-')) return name;
    return name.replace(/[-:]([a-z])/g, (_, letter: string) =>
        letter.toUpperCase(),
    );
}

function reactStyle(value: string): string {
    const declarations = value
        .split(';')
        .map((declaration) => declaration.trim())
        .filter(Boolean)
        .map((declaration) => {
            const separator = declaration.indexOf(':');
            const property = reactAttributeName(
                declaration.slice(0, separator).trim(),
            );
            const propertyValue = declaration.slice(separator + 1).trim();
            return `${JSON.stringify(property)}: ${JSON.stringify(propertyValue)}`;
        });
    return `{{ ${declarations.join(', ')} }}`;
}

function elementToJsx(element: Element, depth: number): string {
    const indentation = '    '.repeat(depth);
    const attributes = [...element.attributes]
        .map((attribute) => {
            if (attribute.name === 'style') {
                return `style=${reactStyle(attribute.value)}`;
            }
            return `${reactAttributeName(attribute.name)}={${JSON.stringify(
                attribute.value,
            )}}`;
        })
        .join(' ');
    const opening = attributes
        ? `<${element.tagName} ${attributes}`
        : `<${element.tagName}`;
    const children = [...element.childNodes].filter(
        (node) =>
            node.nodeType === Node.ELEMENT_NODE ||
            node.textContent?.trim(),
    );

    if (children.length === 0) return `${indentation}${opening} />`;

    const content = children
        .map((node) => {
            if (node instanceof Element) return elementToJsx(node, depth + 1);
            return `${'    '.repeat(depth + 1)}{${JSON.stringify(node.textContent)}}`;
        })
        .join('\n');

    return `${indentation}${opening}>\n${content}\n${indentation}</${element.tagName}>`;
}

function svgToReactComponent(svg: string): string {
    verifySvg(svg);
    const document = new DOMParser().parseFromString(svg, 'image/svg+xml');
    return `export default function AbstractArtwork() {
    return (
${elementToJsx(document.documentElement, 2)}
    );
}
`;
}

async function writeClipboard(text: string): Promise<void> {
    if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(text);
        return;
    }

    const textarea = document.createElement('textarea');
    textarea.value = text;
    textarea.style.position = 'fixed';
    textarea.style.opacity = '0';
    document.body.append(textarea);
    textarea.select();
    const copied = document.execCommand('copy');
    textarea.remove();
    if (!copied) throw new Error('Clipboard access is unavailable.');
}

export default function App() {
    const imageInputRef = useRef<HTMLInputElement>(null);
    const recipeInputRef = useRef<HTMLInputElement>(null);
    const dialsRef = useRef<HTMLDivElement>(null);
    const uploadToken = useRef(0);
    const copyResetTimers = useRef(new Map<string, number>());
    const [initialRecipe] = useState(loadInitialRecipe);
    const [palette, setPalette] = useState(initialRecipe.palette);
    const [variationOffset, setVariationOffset] = useState(
        initialRecipe.variationOffset,
    );
    const [referenceName, setReferenceName] = useState('');
    const [isExtracting, setIsExtracting] = useState(false);
    const [controlsOpen, setControlsOpen] = useState(true);
    const [seedControl, setSeedControl] = useState<HTMLElement | null>(
        null,
    );
    const [status, setStatus] = useState('');
    const [error, setError] = useState('');
    const [copiedActions, setCopiedActions] = useState(
        () => new Set<string>(),
    );
    const dialConfig = useMemo(
        () =>
            createDialConfig(
                initialRecipe,
                referenceName,
                isExtracting,
            ),
        [initialRecipe, isExtracting, referenceName],
    );
    const dial = useDialKitController(
        'Abstract',
        dialConfig,
        {
            id: 'abstract-studio-v2',
            persist: {
                key: DIAL_STORAGE_KEY,
                storage: 'localStorage',
                presets: true,
            },
            onAction: handleDialAction,
        },
    );
    const aspectRatio =
        ASPECT_RATIOS[dial.values.aspectRatio] ??
        ASPECT_RATIOS['16:9'];
    const paletteAdjustments: PaletteAdjustments = useMemo(
        () => ({
            saturation: dial.values.color.saturation,
            hueShift: dial.values.color.hueShift,
            lightness: dial.values.color.lightness,
            contrast: dial.values.color.contrast,
            temperature: dial.values.color.temperature,
            mutedness: dial.values.color.mutedness,
            vintageFade: dial.values.color.vintageFade,
            shadowDepth: dial.values.color.shadowDepth,
        }),
        [dial.values.color],
    );
    const settings: GeneratorSettings = useMemo(
        () => ({
            direction: dial.values.flow.direction,
            flowScale: dial.values.flow.flowScale,
            bend: dial.values.flow.bend,
            vortex: dial.values.flow.vortex,
            density: dial.values.flow.density,
            pathLength: dial.values.flow.pathLength,
            strokeScale: dial.values.flow.strokeScale,
            cropX: dial.values.flow.cropX,
            cropY: dial.values.flow.cropY,
            softness: dial.values.finish.softness,
            motionBlur: dial.values.finish.motionBlur,
            grain: dial.values.finish.grain,
            shadowIntensity: dial.values.finish.shadowIntensity,
        }),
        [dial.values.finish, dial.values.flow],
    );
    const recipe: AbstractRecipe = useMemo(
        () => ({
            schemaVersion: ABSTRACT_SCHEMA_VERSION,
            generatorVersion: ABSTRACT_GENERATOR_VERSION,
            seed: dial.values.seed,
            variationOffset,
            aspectRatio,
            palette,
            paletteAdjustments,
            settings,
        }),
        [
            aspectRatio,
            dial.values.seed,
            palette,
            paletteAdjustments,
            settings,
            variationOffset,
        ],
    );
    const deferredRecipe = useDeferredValue(recipe);
    const svgs = useMemo(
        () =>
            Array.from({ length: 4 }, (_, variation) =>
                generateAbstractSvg(deferredRecipe, variation),
            ),
        [deferredRecipe],
    );

    useEffect(() => {
        document.title = 'Abstract Studio';
    }, []);

    useEffect(() => {
        if (!controlsOpen) {
            setSeedControl(null);
            return;
        }
        const syncSeedControl = () => {
            const control =
                dialsRef.current?.querySelector<HTMLElement>(
                    '.dialkit-text-control',
                ) ?? null;
            setSeedControl((current) =>
                current === control ? current : control,
            );
        };
        const observer = new MutationObserver(syncSeedControl);
        const dials = dialsRef.current;
        const resetSlider = (event: MouseEvent) => {
            const target = event.target;
            if (!(target instanceof Element)) return;
            const slider = target.closest('.dialkit-slider');
            if (!slider || !dials?.contains(slider)) return;
            const label = slider.querySelector(
                '.dialkit-slider-label',
            )?.textContent;
            const reset = label
                ? SLIDER_RESET_VALUES[label]
                : undefined;
            if (!reset) return;
            event.preventDefault();
            dial.setValue(reset.path, reset.value);
            setStatus(`${label} reset to ${reset.value}.`);
        };
        if (dials) {
            observer.observe(dials, {
                childList: true,
                subtree: true,
            });
            dials.addEventListener('dblclick', resetSlider);
        }
        syncSeedControl();
        return () => {
            observer.disconnect();
            dials?.removeEventListener('dblclick', resetSlider);
        };
    }, [controlsOpen]);

    useEffect(
        () => () => {
            for (const timeout of copyResetTimers.current.values()) {
                window.clearTimeout(timeout);
            }
        },
        [],
    );

    useEffect(() => {
        const timeout = window.setTimeout(() => {
            localStorage.setItem(STORAGE_KEY, serializeRecipe(recipe));
        }, 180);
        return () => window.clearTimeout(timeout);
    }, [recipe]);

    function handleDialAction(action: string) {
        if (action === 'uploadReference') {
            imageInputRef.current?.click();
        } else if (action === 'newVariations') {
            setVariationOffset((offset) => offset + 4);
            setStatus('Generated four new variations.');
        } else if (action === 'newVariationsAndHueShift') {
            const hueShift = randomHueShift();
            setVariationOffset((offset) => offset + 4);
            dial.setValues({ color: { hueShift } });
            setStatus(
                `Generated four new variations with hue shift ${hueShift}°.`,
            );
        } else if (action === 'recipe.importRecipe') {
            recipeInputRef.current?.click();
        } else if (action === 'recipe.downloadRecipe') {
            downloadBlob(
                new Blob([serializeRecipe(recipe)], {
                    type: 'application/json;charset=utf-8',
                }),
                `abstract-${safeFilenamePart(recipe.seed)}.json`,
            );
            setStatus('Recipe downloaded.');
        } else if (action === 'recipe.reset') {
            dial.setValues(dialValuesForRecipe(createFactoryRecipe()));
            setVariationOffset(0);
            setStatus('Parameters reset.');
        }
    }

    async function handleReferenceFile(file: File | undefined) {
        if (!file) return;
        if (!['image/jpeg', 'image/png', 'image/webp'].includes(file.type)) {
            setError('Choose a JPG, PNG, or WebP image.');
            return;
        }

        const token = uploadToken.current + 1;
        uploadToken.current = token;
        setIsExtracting(true);
        setError('');
        try {
            const colors = await extractPaletteFromImage(file, 8);
            if (uploadToken.current !== token) return;
            if (colors.length < 5) {
                throw new Error(
                    'This image does not contain enough distinct colors.',
                );
            }
            setPalette(
                colors.map((hex, index) => ({
                    id: `sample-${token}-${index}`,
                    hex,
                    locked: false,
                })),
            );
            setReferenceName(file.name);
            setStatus(`Sampled ${colors.length} colors from ${file.name}.`);
        } catch (caught) {
            setError(
                caught instanceof Error
                    ? caught.message
                    : 'Palette extraction failed.',
            );
        } finally {
            if (uploadToken.current === token) setIsExtracting(false);
        }
    }

    async function importRecipe(event: ChangeEvent<HTMLInputElement>) {
        const file = event.target.files?.[0];
        event.target.value = '';
        if (!file) return;
        try {
            const imported = parseRecipe(await file.text(), recipe);
            setPalette(imported.palette);
            setVariationOffset(imported.variationOffset);
            dial.setValues(dialValuesForRecipe(imported));
            setError('');
            setStatus(`Imported ${file.name}.`);
        } catch (caught) {
            setError(
                caught instanceof Error
                    ? caught.message
                    : 'Recipe import failed.',
            );
        }
    }

    async function copyArtwork(
        svg: string,
        variation: number,
        format: 'SVG' | 'React',
    ) {
        try {
            const content =
                format === 'SVG' ? svg : svgToReactComponent(svg);
            await writeClipboard(content);
            const action = `${recipe.variationOffset + variation}:${format}`;
            const existingTimeout = copyResetTimers.current.get(action);
            if (existingTimeout) window.clearTimeout(existingTimeout);
            setCopiedActions((actions) => {
                const next = new Set(actions);
                next.add(action);
                return next;
            });
            copyResetTimers.current.set(
                action,
                window.setTimeout(() => {
                    setCopiedActions((actions) => {
                        const next = new Set(actions);
                        next.delete(action);
                        return next;
                    });
                    copyResetTimers.current.delete(action);
                }, 1200),
            );
            setError('');
            setStatus(`Variation ${variation + 1} copied as ${format}.`);
        } catch (caught) {
            setError(
                caught instanceof Error
                    ? caught.message
                    : 'Copying failed.',
            );
        }
    }

    return (
        <main className="abstract-studio">
            <h1 className="abstract-studio-visually-hidden">
                Abstract studio
            </h1>
            <input
                ref={imageInputRef}
                type="file"
                accept="image/jpeg,image/png,image/webp"
                className="abstract-studio-visually-hidden"
                aria-label="Upload reference image"
                onChange={(event) => {
                    void handleReferenceFile(event.target.files?.[0]);
                    event.target.value = '';
                }}
            />
            <input
                ref={recipeInputRef}
                type="file"
                accept="application/json,.json"
                className="abstract-studio-visually-hidden"
                aria-label="Import recipe"
                onChange={importRecipe}
            />

            <section
                aria-label="Generated variations"
                className="abstract-studio-grid"
            >
                {svgs.map((svg, index) => {
                    const svgAction = `${variationOffset + index}:SVG`;
                    const reactAction = `${variationOffset + index}:React`;
                    const svgCopied = copiedActions.has(svgAction);
                    const reactCopied = copiedActions.has(reactAction);

                    return (
                        <div
                            key={`${variationOffset}-${index}`}
                            className="abstract-studio-variation"
                        >
                            <img src={svgDataUrl(svg)} alt="" />
                            <div className="abstract-studio-actions">
                                <button
                                    type="button"
                                    className="abstract-studio-action"
                                    aria-label={`Download variation ${index + 1} as SVG`}
                                    title="Download SVG"
                                    onClick={() => {
                                        try {
                                            downloadBlob(
                                                new Blob([svg], {
                                                    type: 'image/svg+xml;charset=utf-8',
                                                }),
                                                `abstract-${safeFilenamePart(recipe.seed)}-${variationOffset + index + 1}.svg`,
                                            );
                                            setError('');
                                            setStatus(
                                                `Variation ${index + 1} downloaded as SVG.`,
                                            );
                                        } catch (caught) {
                                            setError(
                                                caught instanceof Error
                                                    ? caught.message
                                                    : 'SVG download failed.',
                                            );
                                        }
                                    }}
                                >
                                    <Download aria-hidden="true" />
                                </button>
                                <button
                                    type="button"
                                    className="abstract-studio-action"
                                    aria-label={
                                        svgCopied
                                            ? 'SVG copied'
                                            : `Copy variation ${index + 1} as SVG`
                                    }
                                    title="Copy as SVG"
                                    onClick={() => {
                                        void copyArtwork(svg, index, 'SVG');
                                    }}
                                >
                                    <span
                                        className="abstract-studio-action-icon"
                                        data-copied={svgCopied}
                                        aria-hidden="true"
                                    >
                                        <SvgFormat className="abstract-studio-action-default" />
                                        <Check className="abstract-studio-action-check" />
                                    </span>
                                </button>
                                <button
                                    type="button"
                                    className="abstract-studio-action"
                                    aria-label={
                                        reactCopied
                                            ? 'React component copied'
                                            : `Copy variation ${index + 1} as React`
                                    }
                                    title="Copy as React"
                                    onClick={() => {
                                        void copyArtwork(
                                            svg,
                                            index,
                                            'React',
                                        );
                                    }}
                                >
                                    <span
                                        className="abstract-studio-action-icon"
                                        data-copied={reactCopied}
                                        aria-hidden="true"
                                    >
                                        <Code className="abstract-studio-action-default" />
                                        <Check className="abstract-studio-action-check" />
                                    </span>
                                </button>
                            </div>
                        </div>
                    );
                })}
            </section>

            <aside
                aria-label="Generator controls"
                className="abstract-studio-inspector"
                data-open={controlsOpen}
            >
                <button
                    type="button"
                    className="abstract-studio-toggle"
                    aria-expanded={controlsOpen}
                    aria-controls="abstract-studio-dials"
                    onClick={() => setControlsOpen((open) => !open)}
                >
                    {controlsOpen ? (
                        <>
                            <Xmark aria-hidden="true" />
                            <span className="abstract-studio-visually-hidden">
                                Hide controls
                            </span>
                        </>
                    ) : (
                        <>
                            <CurveArray aria-hidden="true" />
                            <span className="abstract-studio-visually-hidden">
                                Show controls
                            </span>
                        </>
                    )}
                </button>
                {controlsOpen ? (
                    <div
                        ref={dialsRef}
                        id="abstract-studio-dials"
                        className="abstract-studio-dials"
                    >
                        <DialRoot mode="inline" theme="dark" />
                        {seedControl
                            ? createPortal(
                                  <button
                                      type="button"
                                      className="abstract-studio-seed-randomize"
                                      aria-label="Randomize seed"
                                      title="Randomize seed"
                                      onClick={() => {
                                          const seed = randomSeedWord();
                                          dial.setValue('seed', seed);
                                          setStatus(
                                              `Changed seed to ${seed}.`,
                                          );
                                      }}
                                  >
                                      <DiceFive aria-hidden="true" />
                                  </button>,
                                  seedControl,
                              )
                            : null}
                    </div>
                ) : null}
            </aside>

            {error ? (
                <p className="abstract-studio-error" role="alert">
                    {error}
                </p>
            ) : null}
            <p
                className="abstract-studio-visually-hidden"
                aria-live="polite"
            >
                {status}
            </p>
        </main>
    );
}
