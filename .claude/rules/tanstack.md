---
paths: ['**/*.{ts,tsx}']
---

# TanStack Start & Router Rules

## TanStack Start (Full-Stack SSR)

**vite.config.ts**:

```typescript
import { tanstackStart } from '@tanstack/react-start/plugin/vite';
import viteReact from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [tanstackStart(), viteReact()]
});
```

**app.config.ts**:

```typescript
import { defineConfig } from '@tanstack/react-start/config';

export default defineConfig({
	server: { preset: 'node-server' } // or 'cloudflare-workers', 'vercel'
});
```

**Scaffold**: `pnpm create @tanstack/start@latest`

## TanStack Router (SPA)

**vite.config.ts**:

```typescript
import { tanstackRouter } from '@tanstack/router-plugin/vite';
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [
		tanstackRouter({ target: 'react', autoCodeSplitting: true }), // BEFORE react()
		react()
	]
});
```

## Router Setup

```typescript
// src/router.tsx
import { createRouter } from '@tanstack/react-router';
import { routeTree } from './routeTree.gen';

export const router = createRouter({
	routeTree,
	defaultPreload: 'intent',
	defaultPreloadStaleTime: 30_000,
	scrollRestoration: true
});

declare module '@tanstack/react-router' {
	interface Register {
		router: typeof router;
	}
}
```

## File-Based Routing

| Pattern      | Description     | Example                            |
| ------------ | --------------- | ---------------------------------- |
| `__root.tsx` | Root layout     | Always required                    |
| `index.tsx`  | Index route     | `/routes/index.tsx` → `/`          |
| `about.tsx`  | Static route    | `/routes/about.tsx` → `/about`     |
| `$param.tsx` | Dynamic param   | `/routes/$postId.tsx` → `/:postId` |
| `$.tsx`      | Catch-all/splat | `/routes/files/$.tsx` → `/files/*` |
| `_layout/`   | Pathless layout | Groups routes without URL segment  |
| `route.tsx`  | Parent layout   | Has `<Outlet />` for children      |
| `-folder/`   | Ignored         | Co-located components/hooks        |

**Flat routes** (dot notation): `posts.$postId.edit.tsx` → `/posts/:postId/edit`
**Directory routes**: `posts/$postId/edit.tsx` → same

## Route File Structure

```typescript
import { createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/posts/$postId')({
  // Parse/validate params
  params: {
    parse: (p) => ({ postId: Number(p.postId) }),
    stringify: (p) => ({ postId: String(p.postId) }),
  },

  // Validate search params (use Zod schema directly)
  validateSearch: z.object({
    tab: z.enum(['details', 'comments']).catch('details'),
    page: z.number().catch(1),
  }),

  // Sequential, blocking — auth checks, redirects, context additions
  beforeLoad: async ({ params, context, location }) => {
    if (!context.auth.user) throw redirect({ to: '/login', search: { redirect: location.pathname } })
    return { user: context.auth.user } // Added to context for children
  },

  // Parallel after ALL beforeLoad complete — data fetching
  loader: async ({ params, context, abortController }) => {
    return fetchPost(params.postId, { signal: abortController.signal })
  },

  // Only these search params trigger loader re-run
  loaderDeps: ({ search }) => ({ tab: search.tab }),

  // Caching
  staleTime: 1000 * 60 * 5,  // 5 min before stale
  gcTime: 1000 * 60 * 30,    // 30 min in memory

  component: PostPage,
  pendingComponent: PostSkeleton,
  errorComponent: PostError,
})

function PostPage() {
  const { postId } = Route.useParams()
  const post = Route.useLoaderData()
  const { tab } = Route.useSearch()
  return <div>{/* ... */}</div>
}
```

## Execution Order (Critical)

1. **beforeLoad** — Sequential parent→child, blocks everything
2. **loader** — Parallel after ALL beforeLoad complete
3. **Component** — After loaders resolve

## Deferred Data

```typescript
import { defer, Await } from '@tanstack/react-router'

export const Route = createFileRoute('/posts/$postId')({
  loader: async ({ params }) => ({
    post: await fetchPost(params.postId),           // Critical — blocks
    comments: defer(fetchComments(params.postId)),  // Non-critical — streams
  }),
  component: () => {
    const { post, comments } = Route.useLoaderData()
    return (
      <>
        <h1>{post.title}</h1>
        <Suspense fallback={<Skeleton />}>
          <Await promise={comments}>{(c) => <Comments data={c} />}</Await>
        </Suspense>
      </>
    )
  },
})
```

## Server Functions (Start Only)

```typescript
import { createServerFn } from '@tanstack/react-start';
import { z } from 'zod';

// GET (default)
export const getUsers = createServerFn().handler(async () => {
	return db.users.findMany();
});

// POST with validation
export const createUser = createServerFn({ method: 'POST' })
	.validator(z.object({ name: z.string(), email: z.string().email() }))
	.handler(async ({ data }) => {
		return db.users.create({ data }); // data is typed
	});

// Access request/response
import { getRequestHeader, setResponseStatus } from '@tanstack/react-start/server';

export const protectedFn = createServerFn({ method: 'POST' }).handler(async () => {
	const auth = getRequestHeader('Authorization');
	setResponseStatus(201);
	return { created: true };
});
```

**Call server functions**:

```typescript
// In loader
loader: () => getUsers();

// In component
const users = await getUsers();

// With useServerFn hook
const createUser = useServerFn(createUserFn);
await createUser({ data: { name, email } });
```

## Middleware

```typescript
import { createMiddleware } from '@tanstack/react-start';

const authMiddleware = createMiddleware({ type: 'function' }).server(async ({ next, context }) => {
	const session = await getSession();
	if (!session) throw new Error('Unauthorized');
	return next({ context: { ...context, user: session.user } });
});

// Composable
const adminMiddleware = createMiddleware({ type: 'function' })
	.middleware([authMiddleware])
	.server(async ({ next, context }) => {
		if (context.user.role !== 'admin') throw new Error('Forbidden');
		return next();
	});

// Apply
export const adminAction = createServerFn({ method: 'POST' })
	.middleware([adminMiddleware])
	.handler(async ({ context }) => {
		/* context.user available */
	});
```

## Authentication Pattern

```typescript
// src/utils/session.ts
import { useSession } from '@tanstack/react-start/server'

export function useAppSession() {
  return useSession<{ userId?: string; role?: string }>({
    password: process.env.SESSION_SECRET!, // 32+ chars
    cookie: { secure: process.env.NODE_ENV === 'production', httpOnly: true, sameSite: 'lax' },
  })
}

// Protected layout: src/routes/_authenticated/route.tsx
export const Route = createFileRoute('/_authenticated')({
  beforeLoad: async ({ location }) => {
    const user = await getCurrentUser()
    if (!user) throw redirect({ to: '/login', search: { redirect: location.pathname } })
    return { user }
  },
  component: () => {
    const { user } = Route.useRouteContext()
    return <><nav>Welcome, {user.name}</nav><Outlet /></>
  },
})
```

## Search Params

```typescript
// Strip defaults from URL
import { stripSearchParams } from '@tanstack/react-router'

export const Route = createFileRoute('/products')({
  validateSearch: z.object({
    page: z.number().catch(1),
    sort: z.enum(['newest', 'price']).catch('newest'),
  }),
  search: { middlewares: [stripSearchParams({ page: 1, sort: 'newest' })] },
})

// Update search params
const navigate = useNavigate()
navigate({ search: (prev) => ({ ...prev, page: 2 }) })

// Or via Link
<Link to="/products" search={(prev) => ({ ...prev, sort: 'price' })}>Sort</Link>
```

## TanStack Query Integration

```typescript
// src/router.tsx
import { routerWithQueryClient } from '@tanstack/react-router-with-query';

export function createAppRouter() {
	const queryClient = new QueryClient();
	return routerWithQueryClient(createRouter({ routeTree, context: { queryClient } }), queryClient);
}

// src/routes/__root.tsx
import { createRootRouteWithContext } from '@tanstack/react-router';
import type { QueryClient } from '@tanstack/react-query';

export const Route = createRootRouteWithContext<{ queryClient: QueryClient }>()({
	component: RootComponent
});

// Query options pattern
export const postsQueryOptions = (page: number) =>
	queryOptions({
		queryKey: ['posts', page],
		queryFn: () => getPosts(page)
	});

// Prefetch in loader (non-blocking)
loader: ({ context, deps }) => {
	context.queryClient.prefetchQuery(postsQueryOptions(deps.page));
};

// Ensure in loader (blocking)
loader: async ({ context, params }) => {
	await context.queryClient.ensureQueryData(postQueryOptions(params.postId));
};

// Use in component
const { data } = useSuspenseQuery(postsQueryOptions(page));
```

## TanStack Form Integration

```typescript
import { useForm } from '@tanstack/react-form'
import { z } from 'zod'

const schema = z.object({
  name: z.string().min(2),
  email: z.string().email(),
})

function MyForm() {
  const form = useForm({
    defaultValues: { name: '', email: '' },
    validators: { onSubmit: schema },
    onSubmit: async ({ value }) => await createUser(value),
  })

  return (
    <form onSubmit={(e) => { e.preventDefault(); form.handleSubmit() }}>
      <form.Field name="name">
        {(field) => (
          <>
            <input
              value={field.state.value}
              onChange={(e) => field.handleChange(e.target.value)}
            />
            {field.state.meta.errors.map((e) => <span key={e}>{e}</span>)}
          </>
        )}
      </form.Field>
      <form.Subscribe selector={(s) => s.canSubmit}>
        {(canSubmit) => <button disabled={!canSubmit}>Submit</button>}
      </form.Subscribe>
    </form>
  )
}
```

## Code Splitting

**Auto** (recommended): `autoCodeSplitting: true` in router plugin config

**Manual** (`.lazy.tsx`):

```typescript
// posts.tsx — critical (loader, beforeLoad, validation)
export const Route = createFileRoute('/posts')({
	loader: () => fetchPosts()
});

// posts.lazy.tsx — lazy loaded (components)
export const Route = createLazyFileRoute('/posts')({
	component: PostsPage,
	pendingComponent: PostsSkeleton
});
```

## Project Structure

```
src/
├── routes/
│   ├── __root.tsx
│   ├── index.tsx
│   ├── _authenticated/        # Protected layout
│   │   ├── route.tsx
│   │   └── dashboard.tsx
│   ├── posts/
│   │   ├── route.tsx
│   │   ├── index.tsx
│   │   ├── $postId.tsx
│   │   └── -components/       # Ignored — co-located
│   └── api/                   # API routes (Start)
├── server/                    # Server functions
├── queries/                   # TanStack Query options
├── components/
├── lib/
├── router.tsx
└── routeTree.gen.ts           # Auto-generated (gitignore)
```

## Common Mistakes

| Mistake                          | Fix                                                     |
| -------------------------------- | ------------------------------------------------------- |
| Router plugin after React plugin | Router plugin MUST come BEFORE react()                  |
| Blocking data in beforeLoad      | Use loader for data; beforeLoad for auth/redirects only |
| Not using loaderDeps             | Specify which search params trigger loader re-run       |
| Missing type registration        | Add `declare module '@tanstack/react-router'`           |
| Committing routeTree.gen.ts      | Add to .gitignore                                       |
| Polling for data                 | Use loader caching + invalidation, not setInterval      |
| Server fn without validation     | Always validate input with `.validator()`               |
| Auth in every route              | Use pathless `_authenticated` layout with beforeLoad    |

## Files to Gitignore

```
src/routeTree.gen.ts
```

## VSCode Settings

```json
{
	"files.readonlyInclude": { "**/routeTree.gen.ts": true },
	"search.exclude": { "**/routeTree.gen.ts": true }
}
```
