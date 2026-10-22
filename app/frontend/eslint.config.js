import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import tseslint from 'typescript-eslint'
import { defineConfig, globalIgnores } from 'eslint/config'

const isSuspense = ":matches(JSXOpeningElement[name.name='Suspense'], JSXOpeningElement[name.property.name='Suspense'])"
const isLazy = ":matches(CallExpression[callee.name='lazy'], CallExpression[callee.property.name='lazy'])"

const loadingRules = [
  ...[
    `${isSuspense} > JSXAttribute[name.name='fallback'] > Literal`,
    `${isSuspense} > JSXAttribute[name.name='fallback'] > JSXExpressionContainer > :not(Literal[raw='null'], JSXElement[openingElement.name.name=/Skeleton$/])`,
  ].map((selector) => ({
    selector,
    message: 'A Suspense fallback is null or the page’s own *Skeleton, never a generic loading screen: a generic fallback flashes before the page’s own skeleton. Prefer importing the page eagerly.',
  })),
  {
    selector: `${isLazy} ImportExpression[source.value=/^(@|\\.\\.?)\\/pages\\//]`,
    message: 'Import pages eagerly: a lazy page renders nothing until its chunk loads, so it cannot show its own skeleton from the first frame.',
  },
  ...['Suspense', 'lazy'].map((name) => ({
    selector: `ImportSpecifier[imported.name='${name}'][local.name!='${name}']`,
    message: `Import ${name} under its own name so the loading rules can see it.`,
  })),
]

const transitionClass = String.raw`/\btransition-(all\b|\[([^\]]*,)?((min|max)-)?(width|height|inset[a-z-]*|top|right|bottom|left)[,\]])/`

const transitionRules = [
  {
    selector: `:matches(Literal[value=${transitionClass}], TemplateElement[value.raw=${transitionClass}])`,
    message: 'List the transitioned properties (transform, opacity, filter) and move size or position through motion’s layout: transitioning everything animates unintended properties, and transitioning size or offsets re-lays out the page every frame.',
  },
]

export default defineConfig([
  globalIgnores(['dist']),
  {
    // Inline disables only warn under noInlineConfig; the lint script's --max-warnings 0 makes them fail.
    linterOptions: { noInlineConfig: true },
  },
  {
    files: ['**/*.{ts,tsx}'],
    extends: [
      js.configs.recommended,
      tseslint.configs.recommended,
      reactHooks.configs.flat.recommended,
      reactRefresh.configs.vite,
    ],
    languageOptions: {
      globals: globals.browser,
    },
    rules: {
      'no-restricted-globals': [
        'error',
        {
          name: 'fetch',
          message: 'Call api() from src/lib/api.ts: it parses the {code, message} error body and throws ApiError, which reauth and form errors depend on.',
        },
      ],
      'no-restricted-properties': [
        'error',
        ...['window', 'globalThis', 'self'].map((object) => ({
          object,
          property: 'fetch',
          message: 'Call api() from src/lib/api.ts: it parses the {code, message} error body and throws ApiError, which reauth and form errors depend on.',
        })),
      ],
      'no-restricted-syntax': ['error', ...loadingRules, ...transitionRules],
      'no-restricted-imports': [
        'error',
        {
          paths: [
            {
              name: 'lucide-react',
              message: 'Replace Lucide icons with local Basicons SVGs so the interface uses one consistent icon style.',
            },
          ],
        },
      ],
    },
  },
  {
    // api() itself, and tests that stub fetch beneath it.
    files: ['src/lib/api.ts', 'src/**/*.test.{ts,tsx}'],
    rules: { 'no-restricted-globals': 'off', 'no-restricted-properties': 'off' },
  },
  {
    // shadcn's generated primitives ship their own transitions.
    files: ['src/components/ui/**'],
    rules: { 'no-restricted-syntax': ['error', ...loadingRules] },
  },
])
