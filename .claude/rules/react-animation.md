---
paths: ['**/*.{tsx,jsx}']
---

# React Animation Rules

## Core Principle: 16.67ms Budget

At 60 FPS, you have **16.67ms per frame**. On 120Hz displays, ~8.33ms. Always design for frame-rate independence.

## Never setTimeout/setInterval for Animation

```typescript
// BAD — not synced with repaint, imprecise timing, runs in background tabs
setInterval(() => updatePosition(), 16);

// GOOD — synced with repaint, pauses when hidden, adapts to refresh rate
function animate(timestamp: number) {
	updatePosition();
	requestAnimationFrame(animate);
}
requestAnimationFrame(animate);
```

## useRef for Animation State, Never useState

Each `useState` update triggers re-render — far too slow for 60 FPS.

```typescript
// BAD — re-renders 60x/second
function AnimatedBox() {
  const [position, setPosition] = useState(0)
  useEffect(() => {
    const animate = () => {
      setPosition(p => p + 1) // Triggers re-render!
      requestAnimationFrame(animate)
    }
    requestAnimationFrame(animate)
  }, [])
  return <div style={{ transform: `translateX(${position}px)` }} />
}

// GOOD — direct DOM manipulation, bypasses React
function AnimatedBox() {
  const positionRef = useRef(0)
  const elementRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const animate = () => {
      positionRef.current += 1
      elementRef.current!.style.transform = `translateX(${positionRef.current}px)`
      requestAnimationFrame(animate)
    }
    requestAnimationFrame(animate)
  }, [])

  return <div ref={elementRef} />
}
```

| Storage     | Re-renders? | Persists? | Use For                            |
| ----------- | ----------- | --------- | ---------------------------------- |
| `const/let` | N/A         | No        | Temporary calculations             |
| `useState`  | Yes         | Yes       | UI state triggering updates        |
| `useRef`    | No          | Yes       | Animation values, DOM refs, timers |

## Complete Animation Pattern

```typescript
function useAnimationFrame(callback: (deltaTime: number) => void) {
  const requestRef = useRef<number>()
  const previousTimeRef = useRef<number>()

  useEffect(() => {
    const animate = (time: number) => {
      if (previousTimeRef.current !== undefined) {
        callback(time - previousTimeRef.current)
      }
      previousTimeRef.current = time
      requestRef.current = requestAnimationFrame(animate)
    }
    requestRef.current = requestAnimationFrame(animate)
    return () => cancelAnimationFrame(requestRef.current!)
  }, [callback])
}

// Usage
function Game() {
  const velocity = useRef({ x: 100, y: 50 }) // px/sec
  const position = useRef({ x: 0, y: 0 })
  const elementRef = useRef<HTMLDivElement>(null)

  useAnimationFrame(useCallback((deltaTime) => {
    const dt = deltaTime / 1000 // Convert to seconds
    position.current.x += velocity.current.x * dt
    position.current.y += velocity.current.y * dt
    elementRef.current!.style.transform =
      `translate(${position.current.x}px, ${position.current.y}px)`
  }, []))

  return <div ref={elementRef} />
}
```

## Delta Time for Frame Independence

```typescript
// BAD — speed varies with frame rate
position += 5; // 60fps=300px/s, 120fps=600px/s, 30fps=150px/s

// GOOD — consistent speed regardless of frame rate
const speed = 0.3; // 300 px/sec
position += speed * deltaTime;
```

## useEffect vs useLayoutEffect

| Hook              | Timing              | Use For                              |
| ----------------- | ------------------- | ------------------------------------ |
| `useEffect`       | After paint (async) | Data fetching, subscriptions         |
| `useLayoutEffect` | Before paint (sync) | DOM measurements, preventing flicker |

```typescript
// BAD — flickers (renders at 0,0 then jumps)
function Tooltip({ targetRef }) {
  const [pos, setPos] = useState({ top: 0, left: 0 })
  useEffect(() => {
    const rect = targetRef.current.getBoundingClientRect()
    setPos({ top: rect.bottom, left: rect.left })
  }, [])
  return <div style={{ position: 'fixed', ...pos }}>Tooltip</div>
}

// GOOD — no flicker (position calculated before paint)
function Tooltip({ targetRef }) {
  const [pos, setPos] = useState({ top: 0, left: 0 })
  useLayoutEffect(() => {
    const rect = targetRef.current.getBoundingClientRect()
    setPos({ top: rect.bottom, left: rect.left })
  }, [])
  return <div style={{ position: 'fixed', ...pos }}>Tooltip</div>
}
```

## GPU-Accelerated Properties

Only `transform` and `opacity` skip Layout/Paint, going directly to GPU compositor:

```typescript
// GOOD — GPU-accelerated, compositor-only
{
	transform: 'translateX(100px)';
}
{
	transform: 'scale(1.5)';
}
{
	transform: 'rotate(45deg)';
}
{
	opacity: 0.5;
}

// BAD — triggers Layout (reflow)
{
	(width, height, top, left, margin, padding, fontSize);
}

// BAD — triggers Paint
{
	(backgroundColor, color, boxShadow, borderRadius);
}
```

| Instead of...      | Use...                   |
| ------------------ | ------------------------ |
| `width`/`height`   | `transform: scale()`     |
| `top`/`left`       | `transform: translate()` |
| `margin`/`padding` | `transform: translate()` |

```css
.animated-element {
	will-change: transform; /* Hint to browser — use sparingly */
}
```

## Avoiding Layout Thrashing

```typescript
// BAD — forces reflow on each iteration
elements.forEach((el) => {
	const width = el.offsetWidth; // Read (forces layout)
	el.style.width = width + 10 + 'px'; // Write
});

// GOOD — batch reads, then batch writes
const widths = elements.map((el) => el.offsetWidth);
elements.forEach((el, i) => {
	el.style.width = widths[i] + 10 + 'px';
});
```

**Properties that force layout**: `offsetTop/Left/Width/Height`, `scrollTop/Left/Width/Height`, `clientTop/Left/Width/Height`, `getBoundingClientRect()`, `getComputedStyle()`

## Animation Libraries

### Framer Motion (Recommended for UI)

```tsx
import { motion, AnimatePresence } from 'framer-motion'

<motion.div
  initial={{ opacity: 0, y: 20 }}
  animate={{ opacity: 1, y: 0 }}
  exit={{ opacity: 0, y: -20 }}
  transition={{ type: 'spring', stiffness: 300, damping: 30 }}
/>

<motion.button whileHover={{ scale: 1.05 }} whileTap={{ scale: 0.95 }} />

<motion.div layout layoutId="shared-element" />
```

### React Spring (Physics-based)

```tsx
import { useSpring, animated } from '@react-spring/web'

const springs = useSpring({
  from: { opacity: 0, transform: 'translateY(20px)' },
  to: { opacity: 1, transform: 'translateY(0px)' },
  config: { tension: 300, friction: 20 },
})

<animated.div style={springs}>Content</animated.div>
```

### GSAP (Complex sequences)

```tsx
import { gsap } from 'gsap';

useEffect(() => {
	const ctx = gsap.context(() => {
		gsap.to(boxRef.current, { x: 100, rotation: 360, duration: 1, ease: 'power2.out' });
	});
	return () => ctx.revert();
}, []);
```

| Feature         | Framer Motion | React Spring | GSAP      |
| --------------- | ------------- | ------------ | --------- |
| Ease of use     | Best          | Good         | Good      |
| Physics         | Great         | Excellent    | Good      |
| Performance     | Great         | Great        | Excellent |
| Exit animations | Built-in      | Manual       | Manual    |
| Timeline        | Basic         | Good         | Excellent |

## Event Handling

```typescript
// BAD — runs 100+ times/second
window.addEventListener('scroll', updateAnimation);

// GOOD — throttle to animation frame rate
let ticking = false;
window.addEventListener(
	'scroll',
	() => {
		if (!ticking) {
			requestAnimationFrame(() => {
				updateAnimation();
				ticking = false;
			});
			ticking = true;
		}
	},
	{ passive: true }
); // Passive = doesn't block scrolling
```

## Reduced Motion Accessibility

```typescript
function usePrefersReducedMotion() {
  const [prefers, setPrefers] = useState(false)
  useEffect(() => {
    const mq = window.matchMedia('(prefers-reduced-motion: reduce)')
    setPrefers(mq.matches)
    const handler = (e: MediaQueryListEvent) => setPrefers(e.matches)
    mq.addEventListener('change', handler)
    return () => mq.removeEventListener('change', handler)
  }, [])
  return prefers
}

// Usage
const prefersReducedMotion = usePrefersReducedMotion()
<motion.div
  animate={{ x: 100 }}
  transition={prefersReducedMotion ? { duration: 0 } : { duration: 0.5 }}
/>
```

```css
@media (prefers-reduced-motion: reduce) {
	*,
	*::before,
	*::after {
		animation-duration: 0.01ms !important;
		transition-duration: 0.01ms !important;
	}
}
```

## Canvas for High-Performance

Use `<canvas>` for thousands of objects, games, or data visualizations:

```tsx
function CanvasAnimation() {
	const canvasRef = useRef<HTMLCanvasElement>(null);
	const particles = useRef<Particle[]>([]);

	useEffect(() => {
		const canvas = canvasRef.current!;
		const ctx = canvas.getContext('2d')!;
		let lastTime = 0;

		const animate = (time: number) => {
			const deltaTime = time - lastTime;
			lastTime = time;

			ctx.clearRect(0, 0, canvas.width, canvas.height);
			particles.current.forEach((p) => {
				p.update(deltaTime);
				p.draw(ctx);
			});

			requestAnimationFrame(animate);
		};
		const id = requestAnimationFrame(animate);
		return () => cancelAnimationFrame(id);
	}, []);

	return <canvas ref={canvasRef} width={800} height={600} />;
}
```

## Common Mistakes

| Mistake                                 | Fix                                            |
| --------------------------------------- | ---------------------------------------------- |
| `useState` for animation values         | Use `useRef` + direct DOM manipulation         |
| `setTimeout`/`setInterval`              | Use `requestAnimationFrame`                    |
| Fixed increment per frame               | Use delta time for frame independence          |
| Animating `width`/`height`/`top`/`left` | Use `transform: translate()`/`scale()`         |
| Reading + writing DOM in alternation    | Batch reads, then batch writes                 |
| Missing cleanup                         | Always `return () => cancelAnimationFrame(id)` |
| Stale closures in animation loop        | Use refs for values accessed in RAF callback   |
| Ignoring reduced motion preference      | Check `prefers-reduced-motion`                 |
| `will-change` everywhere                | Use sparingly — can hurt performance           |
| Heavy work in animation loop            | Offload to Web Workers                         |
