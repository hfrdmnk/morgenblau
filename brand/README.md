# Abstract Studio

A generator for abstract motion-smear brand imagery: one seeded color scene, smeared along one flow field with line integral convolution on the GPU (WebGL2).

```sh
bun install
bun run dev
```

Click the canvas for a new seed, tune the knobs, and press Save. Save keeps the config in this browser's localStorage, where the saved-configs rail reads it, and copies it as JSON. `src/lib/smear-config.ts` owns the config shape and knob bounds; `src/lib/smear-palette.ts` generates each palette from the hue knobs and the seed; `src/lib/smear-renderer.ts` owns the shaders.

Hosted on Vercel with the project's Root Directory set to `brand`. `vercel.json` skips a build when nothing in this folder changed since the last successful deployment.
