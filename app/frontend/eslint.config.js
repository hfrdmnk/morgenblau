import js from '@eslint/js';
import globals from 'globals';
import reactHooks from 'eslint-plugin-react-hooks';
import reactRefresh from 'eslint-plugin-react-refresh';
import tseslint from 'typescript-eslint';
import { defineConfig, globalIgnores } from 'eslint/config';

import comments from './eslint/comments.js';
import designSystem from './eslint/design-system.js';

const isSuspense =
    ":matches(JSXOpeningElement[name.name='Suspense'], JSXOpeningElement[name.property.name='Suspense'])";
const isLazy = ":matches(CallExpression[callee.name='lazy'], CallExpression[callee.property.name='lazy'])";

const loadingRules = [
    ...[
        `${isSuspense} > JSXAttribute[name.name='fallback'] > Literal`,
        `${isSuspense} > JSXAttribute[name.name='fallback'] > JSXExpressionContainer > :not(Literal[raw='null'], JSXElement[openingElement.name.name=/Skeleton$/])`,
    ].map((selector) => ({
        selector,
        message:
            'A Suspense fallback is null or the page’s own *Skeleton, never a generic loading screen: a generic fallback flashes before the page’s own skeleton. Prefer importing the page eagerly.',
    })),
    {
        selector: `${isLazy} ImportExpression[source.value=/^(@|\\.\\.?)\\/pages\\//]`,
        message:
            'Import pages eagerly: a lazy page renders nothing until its chunk loads, so it cannot show its own skeleton from the first frame.',
    },
    ...['Suspense', 'lazy'].map((name) => ({
        selector: `ImportSpecifier[imported.name='${name}'][local.name!='${name}']`,
        message: `Import ${name} under its own name so the loading rules can see it.`,
    })),
];

const transitionClass = String.raw`/\btransition-(all\b|\[([^\]]*,)?((min|max)-)?(width|height|inset[a-z-]*|top|right|bottom|left)[,\]])/`;

const transitionRules = [
    {
        selector: `:matches(Literal[value=${transitionClass}], TemplateElement[value.raw=${transitionClass}])`,
        message:
            'List the transitioned properties (transform, opacity, filter) and move size or position through motion’s layout: transitioning everything animates unintended properties, and transitioning size or offsets re-lays out the page every frame.',
    },
];

const designImports = [
    {
        name: 'cn',
        message:
            'Only src/components/ui/ and src/design/ compose class names; use Frame and Text props or a component variant.',
    },
    { name: 'class-variance-authority', message: 'Variants belong to the components in src/components/ui/.' },
    {
        name: 'wouter',
        importNames: ['Link'],
        message:
            'Use Link from @/components/ui/link: it routes app paths through wouter and takes a variant instead of a class string.',
    },
];

const pulseClass = String.raw`/\banimate-pulse\b/`;

const skeletonRules = [
    {
        selector: `:matches(Literal[value=${pulseClass}], TemplateElement[value.raw=${pulseClass}])`,
        message:
            'Wrap the content in Skeleton from @/components/ui/skeleton instead: a hand-drawn placeholder drifts from the real layout and shifts the page when data arrives.',
    },
];

export default defineConfig([
    globalIgnores(['dist']),
    {
        linterOptions: { noInlineConfig: true },
    },
    {
        files: ['**/*.{js,ts,tsx}'],
        plugins: { comments },
        rules: { 'comments/no-comments': 'error', 'comments/no-eslint-disable': 'error' },
    },
    {
        files: ['**/*.{ts,tsx}'],
        extends: [
            js.configs.recommended,
            tseslint.configs.recommended,
            reactHooks.configs.flat.recommended,
            reactRefresh.configs.vite,
        ],
        plugins: { 'design-system': designSystem },
        languageOptions: {
            globals: globals.browser,
        },
        rules: {
            'no-restricted-globals': [
                'error',
                {
                    name: 'fetch',
                    message:
                        'Call api() from src/lib/api.ts: it parses the {code, message} error body and throws ApiError, which reauth and form errors depend on.',
                },
            ],
            'no-restricted-properties': [
                'error',
                ...['window', 'globalThis', 'self'].map((object) => ({
                    object,
                    property: 'fetch',
                    message:
                        'Call api() from src/lib/api.ts: it parses the {code, message} error body and throws ApiError, which reauth and form errors depend on.',
                })),
            ],
            'no-restricted-syntax': ['error', ...loadingRules, ...transitionRules, ...skeletonRules],
            'design-system/no-raw-elements': 'error',
            'design-system/token-classes': 'error',
            'design-system/no-style-props': 'error',
            'no-restricted-imports': [
                'error',
                {
                    paths: [
                        ...designImports,
                        {
                            name: 'lucide-react',
                            message:
                                'Use the Central icons from src/components/icons.tsx so the interface uses one consistent icon style.',
                        },
                    ],
                    patterns: [
                        {
                            group: ['@central-icons-react/*'],
                            message:
                                'Import icons from src/components/icons.tsx, which owns the Central style variant.',
                        },
                    ],
                },
            ],
        },
    },
    {
        files: ['src/components/icons.tsx'],
        rules: {
            'no-restricted-imports': 'off',
            'design-system/no-style-props': 'off',
            'no-restricted-syntax': ['error', ...loadingRules, ...transitionRules, ...skeletonRules],
            'design-system/token-classes': ['error', { allowDark: true }],
            'react-refresh/only-export-components': ['error', { allowConstantExport: true, extraHOCs: ['icon'] }],
        },
    },
    {
        files: ['src/lib/api.ts', 'src/**/*.test.{ts,tsx}'],
        rules: { 'no-restricted-globals': 'off', 'no-restricted-properties': 'off' },
    },
    {
        files: ['src/design/**'],
        rules: { 'design-system/no-style-props': 'off' },
    },
    {
        files: ['eslint/**'],
        rules: { 'design-system/token-classes': 'off' },
    },
    {
        files: ['src/**/*.test.{ts,tsx}'],
        rules: { 'design-system/no-raw-elements': 'off', 'design-system/no-style-props': 'off' },
    },
    {
        files: ['src/components/ui/**'],
        rules: {
            'no-restricted-syntax': ['error', ...loadingRules],
            'design-system/no-raw-elements': 'off',
            'design-system/no-style-props': 'off',
            'no-restricted-imports': [
                'error',
                {
                    paths: [
                        {
                            name: 'lucide-react',
                            message:
                                'Use the Central icons from src/components/icons.tsx so the interface uses one consistent icon style.',
                        },
                    ],
                    patterns: [
                        {
                            group: ['@central-icons-react/*'],
                            message:
                                'Import icons from src/components/icons.tsx, which owns the Central style variant.',
                        },
                    ],
                },
            ],
        },
    },
]);
