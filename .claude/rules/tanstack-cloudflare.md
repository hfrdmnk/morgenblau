---
paths: ['**/*.{ts,tsx}']
---

# TanStack + Cloudflare Workers Rules

### TanStack Start (SSR) Configuration

**vite.config.ts**:

```typescript
import { defineConfig } from 'vite';
import { cloudflare } from '@cloudflare/vite-plugin';
import { tanstackStart } from '@tanstack/react-start/plugin/vite';
import viteReact from '@vitejs/plugin-react';
import viteTsConfigPaths from 'vite-tsconfig-paths';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
	plugins: [
		cloudflare({ viteEnvironment: { name: 'ssr' } }), // Required for SSR - MUST be before tanstackStart
		viteTsConfigPaths({ projects: ['./tsconfig.json'] }),
		tailwindcss(),
		tanstackStart(),
		viteReact()
	]
});
```

**wrangler.jsonc** (default TanStack entry):

```jsonc
{
	"$schema": "node_modules/wrangler/config-schema.json",
	"name": "my-tanstack-app",
	"compatibility_date": "2025-09-02",
	"compatibility_flags": ["nodejs_compat"],
	"main": "@tanstack/react-start/server-entry",
	"observability": { "enabled": true }
}
```

**Custom worker entry** (for queues, cron, or custom routes):

```jsonc
{
	"main": "src/worker-entry.ts" // Custom entry instead of default
}
```

```typescript
// src/worker-entry.ts — Combine TanStack with queues/cron
import type { Env, QueueMessage } from './lib/types';

export default {
	async fetch(request: Request, env: Env, ctx: ExecutionContext): Promise<Response> {
		const url = new URL(request.url);

		// Custom routes BEFORE TanStack handler
		if (url.pathname.startsWith('/_admin/')) {
			// Auth check, custom logic...
			return Response.json({ ok: true });
		}

		// Serve R2 assets at /r2/*
		if (url.pathname.startsWith('/r2/')) {
			const key = url.pathname.slice(4);
			const object = await env.R2.get(key);
			if (!object) return new Response('Not found', { status: 404 });
			const headers = new Headers();
			object.writeHttpMetadata(headers);
			headers.set('cache-control', 'public, max-age=31536000, immutable');
			return new Response(object.body, { headers });
		}

		// Delegate to TanStack Start
		const { default: handler } = await import('@tanstack/react-start/server-entry');
		return handler.fetch(request, env, ctx);
	},

	async queue(batch: MessageBatch<QueueMessage>, env: Env): Promise<void> {
		for (const msg of batch.messages) {
			try {
				await processMessage(msg.body, env);
				msg.ack();
			} catch {
				msg.retry();
			}
		}
	},

	async scheduled(event: ScheduledEvent, env: Env, ctx: ExecutionContext): Promise<void> {
		// Cron job logic
	}
};
```

**Scaffold**: `npm create cloudflare@latest my-app --framework=tanstack-start`

### TanStack Router (SPA) Configuration

**vite.config.ts**:

```typescript
import { cloudflare } from '@cloudflare/vite-plugin';
import { tanstackRouter } from '@tanstack/router-plugin/vite';
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [tanstackRouter({ target: 'react', autoCodeSplitting: true }), react(), cloudflare()],
	build: {
		target: 'esnext',
		rollupOptions: {
			output: {
				manualChunks: { 'react-vendor': ['react', 'react-dom'], router: ['@tanstack/react-router'] }
			}
		}
	}
});
```

**wrangler.jsonc** (SPA):

```jsonc
{
	"name": "my-spa",
	"compatibility_date": "2025-04-01",
	"assets": { "not_found_handling": "single-page-application" }
}
```

`not_found_handling: "single-page-application"` serves `/index.html` for all non-asset routes.

### Accessing Bindings

**Always use module import** (NOT `getContext("cloudflare")` — issue #3468):

```typescript
import { createServerFn } from '@tanstack/react-start';
import { env } from 'cloudflare:workers';

const getData = createServerFn().handler(async () => {
	const results = await env.DB.prepare('SELECT * FROM users').all();
	return results;
});
```

### Workers vs Pages

| Capability                    | Workers | Pages       |
| ----------------------------- | ------- | ----------- |
| Durable Objects, Queues, Cron | Yes     | No          |
| Static assets                 | Free    | Free        |
| Development trajectory        | Active  | Maintenance |

**Use Workers for new projects** — Pages is converging into Workers.

### D1 Database + Drizzle

**wrangler.jsonc**:

```jsonc
{
	"d1_databases": [
		{
			"binding": "DB",
			"database_name": "my-database",
			"database_id": "<uuid>",
			"migrations_dir": "migrations"
		}
	]
}
```

**Migrations**:

```bash
npx wrangler d1 migrations create my-database initial_schema
npx wrangler d1 migrations apply my-database --local   # Dev
npx wrangler d1 migrations apply my-database --remote  # Prod
```

**Drizzle schema** (`src/db/schema.ts`):

```typescript
import { integer, sqliteTable, text } from 'drizzle-orm/sqlite-core';

export const users = sqliteTable('users', {
	id: integer('id').primaryKey({ autoIncrement: true }),
	email: text('email').unique().notNull(),
	name: text('name'),
	createdAt: integer('created_at', { mode: 'timestamp' }).$defaultFn(() => new Date())
});
```

**Usage**:

```typescript
import { drizzle } from 'drizzle-orm/d1';
const db = drizzle(env.DB);
const allUsers = await db.select().from(users);
```

### R2 Storage

**wrangler.jsonc**:

```jsonc
{ "r2_buckets": [{ "binding": "MY_BUCKET", "bucket_name": "my-bucket-name" }] }
```

**Operations**:

```typescript
// Upload
await env.MY_BUCKET.put(key, fileData, {
	httpMetadata: { contentType: 'image/png' },
	customMetadata: { uploadedBy: 'user-123' }
});

// Download
const object = await env.MY_BUCKET.get(key);
const headers = new Headers();
object.writeHttpMetadata(headers);
return new Response(object.body, { headers });
```

**Presigned URLs** (via S3-compatible API):

```typescript
import { S3Client, PutObjectCommand } from '@aws-sdk/client-s3';
import { getSignedUrl } from '@aws-sdk/s3-request-presigner';

const r2Client = new S3Client({
	region: 'auto',
	endpoint: `https://${ACCOUNT_ID}.r2.cloudflarestorage.com`,
	credentials: { accessKeyId: R2_ACCESS_KEY_ID, secretAccessKey: R2_SECRET }
});

const presignedUrl = await getSignedUrl(
	r2Client,
	new PutObjectCommand({ Bucket: 'my-bucket', Key: key, ContentType: contentType }),
	{ expiresIn: 3600 }
);
```

### Queues

**wrangler.jsonc**:

```jsonc
{
	"queues": {
		"producers": [{ "queue": "my-queue", "binding": "QUEUE" }],
		"consumers": [
			{
				"queue": "my-queue",
				"max_batch_size": 5,
				"max_batch_timeout": 30,
				"max_retries": 3,
				"dead_letter_queue": "my-dlq"
			}
		]
	}
}
```

**Typed queue messages** (`src/lib/types.ts`):

```typescript
// Discriminated union for type-safe queue messages
export type QueueMessage =
	| { type: 'process_submission'; submissionId: number; url: string }
	| { type: 'send_email'; to: string; subject: string }
	| { type: 'generate_thumbnail'; imageKey: string };

export interface Env {
	QUEUE: Queue<QueueMessage>;
	// ... other bindings
}
```

**Producer/Consumer**:

```typescript
// Sending (from server function or custom route)
await env.QUEUE.send({ type: 'process_submission', submissionId: 123, url: 'https://...' })

// Consuming (in worker-entry.ts)
async queue(batch: MessageBatch<QueueMessage>, env: Env): Promise<void> {
  for (const message of batch.messages) {
    try {
      switch (message.body.type) {
        case 'process_submission':
          await processSubmission(message.body.submissionId, message.body.url, env)
          break
        case 'send_email':
          await sendEmail(message.body.to, message.body.subject, env)
          break
      }
      message.ack()
    } catch (error) {
      console.error('Queue error:', error)
      message.retry({ delaySeconds: Math.pow(2, message.attempts) * 10 })
    }
  }
}
```

### Cron Triggers

**wrangler.jsonc**:

```jsonc
{
	"triggers": {
		"crons": ["*/10 * * * *"] // Every 10 minutes
	}
}
```

**Handler** (in worker-entry.ts):

```typescript
async scheduled(event: ScheduledEvent, env: Env, ctx: ExecutionContext): Promise<void> {
  console.log(`[Cron] Running at ${new Date().toISOString()}`)
  // Cleanup, aggregation, deduplication, etc.
}
```

### Workers AI

**wrangler.jsonc**:

```jsonc
{
	"ai": { "binding": "AI" }
}
```

**Usage**:

```typescript
// Text generation
const response = await env.AI.run('@cf/meta/llama-3.1-8b-instruct', {
	prompt: 'Classify this text...',
	max_tokens: 100
});

// Vision (image analysis)
const imageBuffer = await fetch(imageUrl).then((r) => r.arrayBuffer());
const base64 = btoa(String.fromCharCode(...new Uint8Array(imageBuffer)));
const response = await env.AI.run('@cf/meta/llama-3.2-11b-vision-instruct', {
	messages: [
		{
			role: 'user',
			content: [
				{ type: 'text', text: 'Describe this image' },
				{ type: 'image', image: base64 }
			]
		}
	]
});
```

### Multi-Environment Config

**wrangler.toml**:

```toml
name = "my-app"
compatibility_date = "2025-04-01"
compatibility_flags = ["nodejs_compat"]

[vars]
ENVIRONMENT = "development"

[[d1_databases]]
binding = "DB"
database_name = "my-db-dev"
database_id = "<dev-uuid>"

[env.production]
name = "my-app-production"
[env.production.vars]
ENVIRONMENT = "production"
[[env.production.d1_databases]]
binding = "DB"
database_name = "my-db-prod"
database_id = "<prod-uuid>"
[[env.production.routes]]
pattern = "example.com"
custom_domain = true
```

**Secrets** (never in `vars`):

```bash
npx wrangler secret put DATABASE_URL --env production
```

**Local secrets** (`.dev.vars`, gitignore):

```env
API_KEY=dev_secret_key
```

### Custom Domains

```jsonc
{
	"routes": [
		{ "pattern": "api.example.com", "custom_domain": true },
		{ "pattern": "app.example.com", "custom_domain": true }
	]
}
```

SSL/TLS: Full (strict), TLS 1.2+, HSTS enabled.

### Local Development

**Cloudflare Vite plugin uses `workerd` runtime** — production parity.

```bash
npm run dev     # vite dev — Workers runtime locally
npm run build   # vite build
npm run preview # vite preview — test production build
```

**Local service emulation**:

```bash
npx wrangler d1 execute my-db --local --command="SELECT * FROM users"
npx wrangler r2 object put my-bucket/test.txt --file=./test.txt --local
```

Data persists in `.wrangler/state/v3/`. Reset by deleting.

### Testing with Vitest

**vitest.config.ts**:

```typescript
import { defineWorkersConfig, readD1Migrations } from '@cloudflare/vitest-pool-workers/config';
import path from 'node:path';

export default defineWorkersConfig(async () => {
	const migrations = await readD1Migrations(path.join(__dirname, 'migrations'));
	return {
		test: {
			poolOptions: {
				workers: {
					wrangler: { configPath: './wrangler.toml' },
					miniflare: { d1Databases: { DB: { migrations } } },
					isolatedStorage: true
				}
			}
		}
	};
});
```

**Test**:

```typescript
import { env, SELF } from 'cloudflare:test';
import { describe, expect, it } from 'vitest';

describe('API', () => {
	it('returns users from D1', async () => {
		await env.DB.prepare('INSERT INTO users (name) VALUES (?)').bind('Test').run();
		const response = await SELF.fetch('http://localhost/api/users');
		expect(response.status).toBe(200);
	});
});
```

### CI/CD (GitHub Actions)

```yaml
name: Deploy
on:
  push:
    branches: [main, staging]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: pnpm/action-setup@v4
        with: { version: 10 }
      - uses: actions/setup-node@v4
        with: { node-version: 20, cache: 'pnpm' }
      - run: pnpm install --frozen-lockfile && pnpm build
      - uses: cloudflare/wrangler-action@v3
        with:
          apiToken: ${{ secrets.CLOUDFLARE_API_TOKEN }}
          accountId: ${{ secrets.CLOUDFLARE_ACCOUNT_ID }}
          command: deploy ${{ github.ref == 'refs/heads/main' && '--env production' || '--env staging' }}
```

### Project Structure

```
my-tanstack-app/
├── src/
│   ├── routes/
│   │   ├── __root.tsx
│   │   └── index.tsx
│   ├── components/
│   ├── lib/
│   │   ├── types.ts       # Env interface, QueueMessage union
│   │   ├── db.ts          # Database queries
│   │   └── api.ts         # Server functions (createServerFn)
│   ├── server/
│   │   └── queue.ts       # Queue handlers
│   ├── worker-entry.ts    # Custom worker (if using queues/cron)
│   └── routeTree.gen.ts   # Auto-generated (gitignore)
├── migrations/
│   ├── 0001_initial_schema.sql
│   └── 0002_add_indexes.sql
├── public/
├── vite.config.ts
├── wrangler.jsonc
├── .dev.vars              # Secrets for local dev (gitignore)
└── package.json
```

**package.json scripts**:

```json
{
	"scripts": {
		"dev": "vite dev",
		"build": "vite build",
		"preview": "vite preview",
		"deploy": "npm run build && wrangler deploy",
		"typecheck": "tsc --noEmit",
		"cf-typegen": "wrangler types"
	}
}
```

**.gitignore**:

```
.wrangler/
.dev.vars*
src/routeTree.gen.ts
dist/
node_modules/
```

### D1 Performance: Indexes

**Always add indexes** for query patterns to avoid table scans:

```sql
-- Common patterns to index
CREATE INDEX idx_table_status ON table(status);
CREATE INDEX idx_table_created ON table(created_at DESC);
CREATE INDEX idx_table_foreign_key ON table(foreign_key_id);

-- Composite index for filtered + ordered queries
CREATE INDEX idx_table_gallery ON table(status, image_key, id DESC);

-- Partial index for specific conditions (smaller, faster)
CREATE INDEX idx_table_live ON table(status, field) WHERE status = 'live';

-- URL/hash lookups for deduplication
CREATE INDEX idx_table_url ON table(url);
CREATE INDEX idx_table_hash ON table(content_hash);
```

**Migration workflow**:

```bash
# Create migration
npx wrangler d1 migrations create my-db add_indexes

# Apply locally first
npx wrangler d1 migrations apply my-db --local

# Apply to production
npx wrangler d1 migrations apply my-db --remote

# Ad-hoc queries
npx wrangler d1 execute my-db --remote --command "SELECT * FROM table LIMIT 10"
```

### Common Mistakes

| Issue                                       | Fix                                                                |
| ------------------------------------------- | ------------------------------------------------------------------ |
| `getContext("cloudflare")` empty in SSR     | Use `import { env } from "cloudflare:workers"`                     |
| Internal API route calls fail in server fns | Use direct database calls                                          |
| WebSocket packages (Supabase) broken        | Use HTTP-based alternatives or module aliases                      |
| DB drivers requiring WebSockets             | Use Cloudflare-compatible (Neon HTTP, D1)                          |
| Missing `viteEnvironment: { name: 'ssr' }`  | Required for SSR apps — add to cloudflare() plugin                 |
| SPA routes 404                              | Add `not_found_handling: "single-page-application"`                |
| Types out of sync                           | Run `npx wrangler types` after config changes                      |
| Custom routes not working                   | Place custom route handlers BEFORE TanStack import in worker-entry |
| Queue/cron handlers not called              | Use custom `main` entry, not default TanStack entry                |
| Slow D1 queries                             | Add indexes for WHERE/ORDER BY columns                             |
| R2 images not caching                       | Set `cache-control: public, max-age=31536000, immutable`           |

### Env Type Definition

**src/lib/types.ts** — Define all bindings in one place:

```typescript
export interface Env {
	// D1 Database
	DB: D1Database;
	// R2 Storage
	R2: R2Bucket;
	// KV Cache
	KV: KVNamespace;
	// Queue
	QUEUE: Queue<QueueMessage>;
	// Workers AI
	AI: Ai;
	// Environment variables (from vars)
	ENVIRONMENT: string;
	R2_PUBLIC_URL: string;
	// Secrets (from wrangler secret put)
	ADMIN_SECRET: string;
	API_KEY: string;
}
```

### Quick Reference

| Task                       | Command                                                               |
| -------------------------- | --------------------------------------------------------------------- |
| New TanStack Start project | `npm create cloudflare@latest my-app --framework=tanstack-start`      |
| Create D1 migration        | `npx wrangler d1 migrations create DB_NAME migration_name`            |
| Apply migrations (local)   | `npx wrangler d1 migrations apply DB_NAME --local`                    |
| Apply migrations (prod)    | `npx wrangler d1 migrations apply DB_NAME --remote`                   |
| Execute SQL (remote)       | `npx wrangler d1 execute DB_NAME --remote --command "SQL"`            |
| Execute SQL file           | `npx wrangler d1 execute DB_NAME --remote --file=migrations/file.sql` |
| Generate types             | `npx wrangler types`                                                  |
| Local dev                  | `npm run dev` (uses Vite + workerd)                                   |
| Deploy                     | `npx wrangler deploy`                                                 |
| Add secret                 | `npx wrangler secret put SECRET_NAME`                                 |
| List D1 databases          | `npx wrangler d1 list`                                                |
| List R2 buckets            | `npx wrangler r2 bucket list`                                         |
| List KV namespaces         | `npx wrangler kv namespace list`                                      |
