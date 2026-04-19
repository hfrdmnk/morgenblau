import js from '@eslint/js';
import tseslint from 'typescript-eslint';
import reactHooks from 'eslint-plugin-react-hooks';
import reactRefresh from 'eslint-plugin-react-refresh';

export default tseslint.config(
	{ ignores: ['dist/', '.output/', 'src/routeTree.gen.ts'] },
	js.configs.recommended,
	...tseslint.configs.recommended,
	{
		plugins: {
			'react-hooks': reactHooks,
			'react-refresh': reactRefresh
		},
		rules: {
			...reactHooks.configs.recommended.rules,
			'react-refresh/only-export-components': [
				'warn',
				{
					allowConstantExport: true,
					allowExportNames: ['useAuth', 'useSurfaceLevel', 'buttonVariants']
				}
			]
		}
	},
	{
		files: ['src/routes/**/*.tsx', 'src/router.tsx'],
		rules: {
			'react-refresh/only-export-components': 'off'
		}
	}
);
