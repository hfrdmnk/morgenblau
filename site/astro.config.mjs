// @ts-check
import { defineConfig } from 'astro/config';

// https://astro.build/config
export default defineConfig({
	server: { allowedHosts: process.env.AMP_ORB ? true : undefined },
});
