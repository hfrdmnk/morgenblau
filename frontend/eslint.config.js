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
