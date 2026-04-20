import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { defineConfig, type Plugin } from 'vite';
import tsConfigPaths from 'vite-tsconfig-paths';
import { tanstackStart } from '@tanstack/react-start/plugin/vite';
import tailwindcss from '@tailwindcss/vite';
import viteReact from '@vitejs/plugin-react';

const SERVER_HOST = '127.0.0.1';
const SERVER_PORT = 3000;

type OAuthClientMetadata = {
	client_id: string;
	redirect_uris: [string, ...string[]];
	scope: string;
};

function oauthEnv(): Plugin {
	return {
		name: 'morgenblau-oauth-env',
		config(_conf, { command }) {
			const raw = readFileSync(resolve(__dirname, 'public/oauth-client-metadata.json'), 'utf8');
			const metadata = JSON.parse(raw) as OAuthClientMetadata;

			let clientId: string;
			let redirectUri: string;
			if (command === 'build') {
				clientId = metadata.client_id;
				redirectUri = metadata.redirect_uris[0];
			} else {
				redirectUri = `http://${SERVER_HOST}:${SERVER_PORT}${new URL(metadata.redirect_uris[0]).pathname}`;
				clientId =
					`http://localhost?redirect_uri=${encodeURIComponent(redirectUri)}` +
					`&scope=${encodeURIComponent(metadata.scope)}`;
			}

			return {
				define: {
					'import.meta.env.VITE_OAUTH_CLIENT_ID': JSON.stringify(clientId),
					'import.meta.env.VITE_OAUTH_REDIRECT_URI': JSON.stringify(redirectUri),
					'import.meta.env.VITE_OAUTH_SCOPE': JSON.stringify(metadata.scope)
				}
			};
		}
	};
}

export default defineConfig({
	server: { port: SERVER_PORT, host: SERVER_HOST },
	plugins: [
		oauthEnv(),
		tailwindcss(),
		tsConfigPaths(),
		tanstackStart(),
		viteReact() // must come after tanstackStart
	]
});
