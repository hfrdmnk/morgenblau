---
paths: ['**/*.{tsx,jsx}']
---

# React 19 + TypeScript + Base UI Rules

**Note: We do NOT use React Server Components. All components are client components.**

### React 19 Changes

- **ref as prop** — no more `forwardRef`
- **React Compiler** — auto-memoization; remove manual `useMemo`/`useCallback`/`memo` if enabled
- **Context shorthand** — `<Context value={}>` instead of `<Context.Provider>`
- **Document metadata** — `<title>`, `<meta>`, `<link>` hoist to `<head>`

**New Hooks**:

```typescript
const [state, submitAction, isPending] = useActionState(serverAction, initialState);
const [optimisticName, setOptimisticName] = useOptimistic(currentName);
const { pending, data } = useFormStatus(); // Must be in child of <form>
const data = use(dataPromise); // Read promises/context conditionally

const deferredValue = useDeferredValue(expensiveValue); // Requires memo on consumer
const deferredValue = useDeferredValue(expensiveValue, initialValue); // React 19

const [isPending, startTransition] = useTransition();
startTransition(() => setFilteredItems(filter(items))); // Non-urgent updates
```

### Base UI (Headless Components)

**Package**: `@base-ui/react` — unstyled, accessible components from MUI

**Import Pattern**:

```typescript
import { Dialog } from '@base-ui/react/dialog';
import { Tabs } from '@base-ui/react/tabs';
import { Menu } from '@base-ui/react/menu';
import { Switch } from '@base-ui/react/switch';
import { Slider } from '@base-ui/react/slider';
import { Progress } from '@base-ui/react/progress';
import { Collapsible } from '@base-ui/react/collapsible';
import { AlertDialog } from '@base-ui/react/alert-dialog';
import { Checkbox } from '@base-ui/react/checkbox';
import { Popover } from '@base-ui/react/popover';
import { Tooltip } from '@base-ui/react/tooltip';
import { Select } from '@base-ui/react/select';
```

**Common Components**:

```typescript
// Dialog
<Dialog.Root open={open} onOpenChange={setOpen}>
  <Dialog.Trigger>Open</Dialog.Trigger>
  <Dialog.Portal>
    <Dialog.Backdrop className="fixed inset-0 bg-black/50" />
    <Dialog.Popup className="fixed ...">
      <Dialog.Title>Title</Dialog.Title>
      <Dialog.Description>Content</Dialog.Description>
      <Dialog.Close>Close</Dialog.Close>
    </Dialog.Popup>
  </Dialog.Portal>
</Dialog.Root>

// Tabs
<Tabs.Root value={tab} onValueChange={setTab}>
  <Tabs.List><Tabs.Tab value="a">A</Tabs.Tab></Tabs.List>
  <Tabs.Panel value="a">Content A</Tabs.Panel>
</Tabs.Root>

// Menu
<Menu.Root>
  <Menu.Trigger>Menu</Menu.Trigger>
  <Menu.Portal>
    <Menu.Positioner>
      <Menu.Popup>
        <Menu.Item>Item 1</Menu.Item>
      </Menu.Popup>
    </Menu.Positioner>
  </Menu.Portal>
</Menu.Root>

// Switch
<Switch.Root checked={on} onCheckedChange={setOn}>
  <Switch.Thumb />
</Switch.Root>

// Slider
<Slider.Root value={[50]} onValueChange={setValue}>
  <Slider.Control>
    <Slider.Track>
      <Slider.Indicator />
      <Slider.Thumb />
    </Slider.Track>
  </Slider.Control>
</Slider.Root>
```

**Base UI Best Practices**:

- All styling via className — components are unstyled
- Use Tailwind classes directly on Base UI components
- Portal components (Dialog, Menu) need Portal wrapper for proper z-index
- Compose sub-components for full accessibility
- Never style the Root — style child components instead

### useEffect Rules (Critical)

**NEVER Use useEffect For:**

- **Derived state** — `const fullName = first + ' ' + last;` (not useEffect + setState)
- **Event responses** — `function handleSubmit() { submit(); showToast(); }` (not useEffect)
- **Reset on prop change** — `<Profile key={userId} />` (not useEffect)

**DO Use useEffect For:** External sync (WebSocket, DOM APIs), subscriptions, data fetching (with cleanup)

**Race Condition Handling:**

```typescript
// Pattern 1: Boolean flag (simple, works everywhere)
useEffect(() => {
	let ignore = false;
	fetchData(id).then((data) => {
		if (!ignore) setData(data);
	});
	return () => {
		ignore = true;
	};
}, [id]);

// Pattern 2: AbortController (cancels request, saves bandwidth)
useEffect(() => {
	const controller = new AbortController();
	fetch(url, { signal: controller.signal })
		.then((res) => res.json())
		.then(setData)
		.catch((e) => {
			if (e.name !== 'AbortError') throw e;
		});
	return () => controller.abort();
}, [url]);
```

| Pattern         | Pros                       | Cons                                   |
| --------------- | -------------------------- | -------------------------------------- |
| Boolean flag    | Simple, universal          | Wasted requests still complete         |
| AbortController | Cancels in-flight requests | Requires error handling for AbortError |

**Always Cleanup:** `return () => sub.unsubscribe();`

**useLayoutEffect**: Only for DOM measurements before paint.

### Preventing Unnecessary Renders

**With React Compiler**: Write plain code — compiler handles memoization.

**Without Compiler**: `React.memo` for expensive components; `useMemo` for expensive calcs (>1ms); `useCallback` when passed to memoized children

**Reference equality breaks memo:**

```typescript
// Bad: <Child config={{ theme: 'dark' }} />
// Good: const CONFIG = { theme: 'dark' };  // Hoist static
// Good: const config = useMemo(() => ({ theme }), [theme]);  // Memoize dynamic
```

**Composition pattern** (children don't re-render when Parent state changes):

```typescript
function Parent({ children }) {
  const [count, setCount] = useState(0);
  return <div>{count}{children}</div>;
}
<Parent><ExpensiveChild /></Parent>
```

**Provider isolation pattern** (isolate hooks that subscribe to frequently-changing data):

```typescript
// Problem — useHotkeys subscribes to data that updates every 2 seconds,
// causing App to re-render even though it doesn't use that data directly
function App() {
  useHotkeys();  // Subscribes to process queries internally
  return <div>...</div>;  // Re-renders every 2 seconds!
}

// Solution — extract into provider, App becomes a child that doesn't re-render
function HotkeysProvider({ children }: { children: React.ReactNode }) {
  useHotkeys();  // Re-renders here (cheap, no visual output)
  return <>{children}</>;  // App doesn't re-render
}

// In main.tsx
<HotkeysProvider>
  <App />  // Isolated from useHotkeys re-renders
</HotkeysProvider>
```

**Why it works:** When a parent re-renders, React doesn't re-render children passed as props — they're already rendered React elements.

### TypeScript Patterns

```typescript
type Props = { label: string; onClick: () => void; disabled?: boolean };
function Button({ label, onClick, disabled = false }: Props) { }

const [status, setStatus] = useState<'idle' | 'loading' | 'error'>('idle');
const [user, setUser] = useState<User | null>(null);
const [items, setItems] = useState<Item[]>([]);

const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {};
const inputRef = useRef<HTMLInputElement>(null);

// React 19 ref as prop
function Input({ ref, ...props }: React.ComponentPropsWithRef<'input'>) {
  return <input ref={ref} {...props} />;
}

// Generic components
const List = <T extends { id: string },>({ items, render }: {
  items: T[]; render: (item: T) => ReactNode;
}) => <ul>{items.map(i => <li key={i.id}>{render(i)}</li>)}</ul>;

// Context with null check
const Ctx = createContext<CtxType | null>(null);
function useCtx() {
  const ctx = useContext(Ctx);
  if (!ctx) throw new Error('Missing provider');
  return ctx;
}
```

### State Management

| useState      | useReducer                                      |
| ------------- | ----------------------------------------------- |
| Simple values | Complex/related state, next depends on previous |

**Lazy init:** `useState(() => expensiveComputation())` not `useState(expensiveComputation())`

**Never store derived state** — compute during render or useMemo if expensive.

### Performance

**Keys**: Stable unique IDs. Index is fine for static lists that won't reorder. NEVER index for dynamic lists. NEVER Math.random().

**Code splitting:** `const Heavy = lazy(() => import('./Heavy')); <Suspense fallback={<Loading />}><Heavy /></Suspense>`

**Virtualization**: `@tanstack/react-virtual` for 100+ items.

**Concurrent rendering:**

```typescript
const deferredResults = useDeferredValue(searchResults);
const isStale = searchResults !== deferredResults;
<Results data={deferredResults} style={{ opacity: isStale ? 0.7 : 1 }} />  // MUST use React.memo

const [isPending, startTransition] = useTransition();
setInputValue(e.target.value);  // High priority
startTransition(() => setSearchQuery(e.target.value));  // Low priority
```

**Context optimization**: Split by update frequency, memoize provider values.

### Testing (React Testing Library)

**Query priority**: `getByRole` → `getByLabelText` → `getByText` → `getByTestId`
**Query types**: `getBy` (exists), `queryBy` (not exists), `findBy` (async)

`await userEvent.click(button); await waitFor(() => expect(screen.getByText('Done')).toBeInTheDocument());`

### Common Mistakes

- Never useEffect for: derived state, event responses, reset on prop change
- Never: data values in keys (`key={item.name}`), index as key for dynamic lists (ok for static), inline objects breaking memo, mutate state directly, store computable values
- Never: manually memo with Compiler enabled, useDeferredValue without memo on consumer
- Never: schedule async work (setTimeout/setInterval) inside state updaters — React batches them, causing ticks to pile up
- Never: put hooks that subscribe to high-frequency data in App.tsx — use provider isolation pattern
- Always: useEffect cleanup, include all dependencies, stable keys

### useEffect Decision Tree

```
Need useEffect?
├─ Responding to event? → Event handler
├─ Computing from props/state? → Calculate during render
├─ Resetting state on prop change? → Use key prop
├─ Syncing with external system? → YES, useEffect + cleanup
└─ None of above? → Probably don't need useEffect
```
