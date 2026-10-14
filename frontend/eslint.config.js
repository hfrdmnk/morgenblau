import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import tseslint from 'typescript-eslint'
import { defineConfig, globalIgnores } from 'eslint/config'

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
      'no-restricted-syntax': [
        'error',
        ...[
          ":matches(JSXOpeningElement[name.name='Suspense'], JSXOpeningElement[name.property.name='Suspense']) > JSXAttribute[name.name='fallback'] > Literal",
          ":matches(JSXOpeningElement[name.name='Suspense'], JSXOpeningElement[name.property.name='Suspense']) > JSXAttribute[name.name='fallback'] > JSXExpressionContainer > :not(Literal[raw='null'], JSXElement[openingElement.name.name=/Skeleton$/])",
        ].map((selector) => ({
          selector,
          message: 'A Suspense fallback is null or the page’s own *Skeleton, never a generic loading screen. Prefer importing the page eagerly.',
        })),
        {
          selector: "ImportSpecifier[imported.name='Suspense'][local.name!='Suspense']",
          message: 'Import Suspense under its own name so the fallback rule can see it.',
        },
      ],
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
])
