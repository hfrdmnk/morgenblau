---
paths: ['**/*.{tsx,jsx}']
---

# Base UI Rules

Base UI v1.0.0 (`@base-ui/react`) is a headless React component library providing accessible, unstyled primitives. Created by contributors from Radix, Floating UI, and Material UI, it delivers behavior, accessibility, and state management without styling opinions. The legacy `@mui/base` package is deprecated—all new implementations should use `@base-ui/react`.

## Installation and package configuration

Install the current stable package:

```bash
npm i @base-ui/react
# or
yarn add @base-ui/react
# or
pnpm add @base-ui/react
```

**Peer dependencies**: React 17, 18, or 19. No additional styling dependencies required.

**TypeScript requirements**: Minimum version 4.9. Enable `strict` mode in tsconfig.json for best type inference:

```json
{
	"compilerOptions": {
		"lib": ["es6", "dom"],
		"strict": true,
		"allowSyntheticDefaultImports": true
	}
}
```

**Tree-shaking imports** (recommended):

```tsx
import { Popover } from '@base-ui/react/popover';
import { Button } from '@base-ui/react/button';
import { Dialog } from '@base-ui/react/dialog';
```

## Compound components architecture

Base UI uses a **compound components pattern** where complex components are assembled from discrete subcomponents under a namespace. Each part renders a specific DOM node and receives its own props.

```tsx
import { Menu } from '@base-ui/react/menu';

<Menu.Root>
	<Menu.Trigger>Open</Menu.Trigger>
	<Menu.Portal>
		<Menu.Positioner sideOffset={8}>
			<Menu.Popup>
				<Menu.Item>Option 1</Menu.Item>
				<Menu.Item>Option 2</Menu.Item>
				<Menu.Separator />
				<Menu.Item>Option 3</Menu.Item>
			</Menu.Popup>
		</Menu.Positioner>
	</Menu.Portal>
</Menu.Root>;
```

**Standard subcomponent roles**:

- **`.Root`** — provides context and state management
- **`.Trigger`** — handles user interaction to open/close
- **`.Portal`** — renders content outside DOM hierarchy
- **`.Positioner`** — handles floating element positioning
- **`.Popup`** — the actual popup content container

### The render prop for composition

Base UI uses a `render` prop (not `asChild` like Radix) to compose with custom components or change the rendered element:

```tsx
// Change default element type
<Menu.Item render={<a href="/profile" />}>
  Profile
</Menu.Item>

// Compose with custom component
<Menu.Trigger render={<MyButton variant="primary" />}>
  Open Menu
</Menu.Trigger>

// Function form with state access
<Switch.Thumb
  render={(props, state) => (
    <span {...props}>
      {state.checked ? <CheckIcon /> : <CloseIcon />}
    </span>
  )}
/>
```

**Critical requirement**: Custom components used with `render` must forward refs and spread all props:

```tsx
const MyButton = React.forwardRef<HTMLButtonElement, ButtonProps>((props, ref) => (
	<button ref={ref} {...props} className={`custom ${props.className}`} />
));
```

### Nested composition pattern

For elements that need to be triggers for multiple components simultaneously:

```tsx
<Dialog.Root>
	<Tooltip.Root>
		<Tooltip.Trigger
			render={
				<Dialog.Trigger render={<Menu.Trigger render={<MyButton />}>Multi-trigger</Menu.Trigger>} />
			}
		/>
	</Tooltip.Root>
</Dialog.Root>
```

## Styling approaches

Base UI components render with zero CSS by default. Style using any approach: plain CSS with data attributes, CSS Modules, Tailwind CSS, or CSS-in-JS.

### Data attributes for state-based styling

Components expose data attributes reflecting internal state:

| Attribute                                       | Description                    |
| ----------------------------------------------- | ------------------------------ |
| `[data-checked]` / `[data-unchecked]`           | Toggle states                  |
| `[data-open]` / `[data-closed]`                 | Open/close states              |
| `[data-highlighted]`                            | Menu/select item focus         |
| `[data-selected]`                               | Selected item                  |
| `[data-disabled]`                               | Disabled state                 |
| `[data-invalid]` / `[data-valid]`               | Form validation                |
| `[data-dirty]` / `[data-touched]`               | Form field interaction         |
| `[data-dragging]`                               | Slider thumb dragging          |
| `[data-starting-style]` / `[data-ending-style]` | Animation states               |
| `[data-popup-open]`                             | Trigger when its popup is open |

```css
.SwitchThumb[data-checked] {
	background-color: #22c55e;
	transform: translateX(20px);
}

.MenuItem[data-highlighted] {
	background-color: #1f2937;
	color: white;
}

.Input[data-invalid] {
	border-color: #ef4444;
}
```

### className and style as functions

Both `className` and `style` props accept functions receiving component state:

```tsx
<Switch.Thumb
  className={(state) => `thumb ${state.checked ? 'checked' : 'unchecked'}`}
  style={(state) => ({
    backgroundColor: state.checked ? '#22c55e' : '#9ca3af'
  })}
/>

<Toggle
  className={(state) => `toggle ${state.pressed ? 'pressed' : ''}`}
/>
```

### Tailwind CSS integration

Apply Tailwind classes directly, using `data-*` variants for states:

```tsx
<Menu.Trigger className="flex h-10 items-center justify-center gap-1.5 rounded-md border border-gray-200 bg-gray-50 px-3.5 text-base font-medium text-gray-900 hover:bg-gray-100 focus-visible:outline focus-visible:outline-2 focus-visible:-outline-offset-1 focus-visible:outline-blue-800 data-[popup-open]:bg-gray-100">
  Menu
</Menu.Trigger>

<Menu.Item className="flex cursor-default py-2 pr-8 pl-4 text-sm outline-none select-none data-[highlighted]:bg-gray-900 data-[highlighted]:text-white">
  Add to Library
</Menu.Item>
```

### CSS-in-JS (Emotion/styled-components)

```tsx
import styled from '@emotion/styled';
import { Menu } from '@base-ui/react/menu';

const StyledMenuItem = styled(Menu.Item)`
	padding: 0.5rem 1rem;
	cursor: default;

	&[data-highlighted] {
		background-color: #1f2937;
		color: white;
	}
`;
```

### CSS variables

Components expose CSS variables for dynamic positioning values:

```css
.PopoverPopup {
	max-height: var(--available-height);
	min-width: var(--anchor-width);
}
```

## Component patterns and props

### Dialog

```tsx
import { Dialog } from '@base-ui/react/dialog';

<Dialog.Root open={open} onOpenChange={setOpen}>
	<Dialog.Trigger>Open</Dialog.Trigger>
	<Dialog.Portal>
		<Dialog.Backdrop className="backdrop" />
		<Dialog.Popup className="popup" initialFocus={inputRef}>
			<Dialog.Title>Dialog Title</Dialog.Title>
			<Dialog.Description>Dialog content goes here.</Dialog.Description>
			<Dialog.Close>Close</Dialog.Close>
		</Dialog.Popup>
	</Dialog.Portal>
</Dialog.Root>;
```

**Key props**:

| Prop           | Type                               | Default | Description               |
| -------------- | ---------------------------------- | ------- | ------------------------- |
| `modal`        | `boolean \| 'trap-focus'`          | `true`  | Modal behavior            |
| `open`         | `boolean`                          | —       | Controlled open state     |
| `onOpenChange` | `(open: boolean, details) => void` | —       | State change handler      |
| `initialFocus` | `RefObject \| function`            | —       | Element to focus on open  |
| `finalFocus`   | `RefObject \| function`            | —       | Element to focus on close |

**Automatic accessibility**: `role="dialog"`, `aria-modal="true"`, `aria-labelledby` linked to Title, `aria-describedby` linked to Description, focus trapping, scroll lock, Escape key closes.

### Menu

```tsx
import { Menu } from '@base-ui/react/menu';

<Menu.Root>
	<Menu.Trigger>Options</Menu.Trigger>
	<Menu.Portal>
		<Menu.Positioner sideOffset={8}>
			<Menu.Popup>
				<Menu.Item>Edit</Menu.Item>
				<Menu.Item>Duplicate</Menu.Item>
				<Menu.Separator />
				<Menu.CheckboxItem checked={checked} onCheckedChange={setChecked}>
					Show Hidden
				</Menu.CheckboxItem>
				<Menu.RadioGroup value={sort} onValueChange={setSort}>
					<Menu.RadioItem value="name">Sort by Name</Menu.RadioItem>
					<Menu.RadioItem value="date">Sort by Date</Menu.RadioItem>
				</Menu.RadioGroup>
				<Menu.SubmenuRoot>
					<Menu.SubmenuTrigger>More</Menu.SubmenuTrigger>
					<Menu.Portal>
						<Menu.Positioner>
							<Menu.Popup>
								<Menu.Item>Nested Item</Menu.Item>
							</Menu.Popup>
						</Menu.Positioner>
					</Menu.Portal>
				</Menu.SubmenuRoot>
			</Menu.Popup>
		</Menu.Positioner>
	</Menu.Portal>
</Menu.Root>;
```

**Key props**:

| Prop                   | Type                         | Default      | Description                    |
| ---------------------- | ---------------------------- | ------------ | ------------------------------ |
| `orientation`          | `'vertical' \| 'horizontal'` | `'vertical'` | Arrow key navigation direction |
| `openOnHover`          | `boolean`                    | `false`      | Open on hover                  |
| `delay` / `closeDelay` | `number`                     | —            | Hover delays in ms             |
| `closeOnClick` (Item)  | `boolean`                    | `true`       | Close menu on item click       |
| `label` (Item)         | `string`                     | —            | Typeahead label                |

**Automatic keyboard**: Arrow Up/Down navigate, Home/End jump to first/last, Enter/Space activate, Escape close, alphanumeric typeahead.

### Select

```tsx
import { Select } from '@base-ui/react/select';

<Select.Root value={value} onValueChange={setValue}>
	<Select.Trigger>
		<Select.Value placeholder="Select option" />
		<Select.Icon />
	</Select.Trigger>
	<Select.Portal>
		<Select.Positioner>
			<Select.Popup>
				<Select.Item value="option1">
					<Select.ItemIndicator>✓</Select.ItemIndicator>
					<Select.ItemText>Option 1</Select.ItemText>
				</Select.Item>
				<Select.Group>
					<Select.GroupLabel>Group</Select.GroupLabel>
					<Select.Item value="option2">
						<Select.ItemText>Option 2</Select.ItemText>
					</Select.Item>
				</Select.Group>
			</Select.Popup>
		</Select.Positioner>
	</Select.Portal>
</Select.Root>;
```

**Key props**:

| Prop            | Type                       | Default | Description          |
| --------------- | -------------------------- | ------- | -------------------- |
| `value`         | `Value \| Value[]`         | —       | Controlled value     |
| `defaultValue`  | `Value \| Value[]`         | —       | Uncontrolled default |
| `onValueChange` | `(value, details) => void` | —       | Value change handler |
| `multiple`      | `boolean`                  | `false` | Multiple selection   |
| `required`      | `boolean`                  | `false` | Form validation      |

### Tabs

```tsx
import { Tabs } from '@base-ui/react/tabs';

<Tabs.Root defaultValue="tab1">
	<Tabs.List>
		<Tabs.Tab value="tab1">Tab 1</Tabs.Tab>
		<Tabs.Tab value="tab2">Tab 2</Tabs.Tab>
		<Tabs.Indicator />
	</Tabs.List>
	<Tabs.Panel value="tab1">Content 1</Tabs.Panel>
	<Tabs.Panel value="tab2">Content 2</Tabs.Panel>
</Tabs.Root>;
```

**Key props**:

| Prop                     | Type                         | Default        | Description                     |
| ------------------------ | ---------------------------- | -------------- | ------------------------------- |
| `value` / `defaultValue` | `string`                     | —              | Active tab identifier           |
| `onValueChange`          | `(value) => void`            | —              | Tab change handler              |
| `orientation`            | `'horizontal' \| 'vertical'` | `'horizontal'` | Layout direction                |
| `activateOnFocus`        | `boolean`                    | `false`        | Activate tab on arrow key focus |

### Slider

```tsx
import { Slider } from '@base-ui/react/slider';

<Slider.Root defaultValue={50} min={0} max={100} step={1}>
  <Slider.Value />
  <Slider.Control>
    <Slider.Track>
      <Slider.Indicator />
      <Slider.Thumb />
    </Slider.Track>
  </Slider.Control>
</Slider.Root>

// Range slider
<Slider.Root defaultValue={[25, 75]}>
  <Slider.Control>
    <Slider.Track>
      <Slider.Indicator />
      <Slider.Thumb index={0} />
      <Slider.Thumb index={1} />
    </Slider.Track>
  </Slider.Control>
</Slider.Root>
```

**Key props**: `value`, `defaultValue`, `onValueChange`, `min`, `max`, `step`, `largeStep` (Page Up/Down), `orientation`, `thumbCollisionBehavior` (`'push' | 'swap' | 'none'`).

### Switch and Checkbox

```tsx
import { Switch } from '@base-ui/react/switch';
import { Checkbox } from '@base-ui/react/checkbox';

<Switch.Root checked={checked} onCheckedChange={setChecked}>
  <Switch.Thumb />
</Switch.Root>

<Checkbox.Root checked={checked} onCheckedChange={setChecked}>
  <Checkbox.Indicator>✓</Checkbox.Indicator>
</Checkbox.Root>
```

**Key props**: `checked`, `defaultChecked`, `onCheckedChange`, `disabled`, `required`, `name`, `value`.

Checkbox supports `checked: boolean | 'indeterminate'`.

## State management patterns

### Controlled vs uncontrolled

Components are **uncontrolled by default**, managing internal state. Use controlled mode by passing value props with change handlers:

| Component Type | Controlled Props              | Uncontrolled Props |
| -------------- | ----------------------------- | ------------------ |
| Value inputs   | `value` + `onValueChange`     | `defaultValue`     |
| Boolean states | `checked` + `onCheckedChange` | `defaultChecked`   |
| Open states    | `open` + `onOpenChange`       | `defaultOpen`      |
| Pressed states | `pressed` + `onPressedChange` | `defaultPressed`   |

```tsx
// Uncontrolled
<Slider.Root defaultValue={50}>...</Slider.Root>;

// Controlled
const [value, setValue] = useState(50);
<Slider.Root value={value} onValueChange={setValue}>
	...
</Slider.Root>;
```

### Event details and cancellation

Change handlers receive a second `eventDetails` argument:

```tsx
<Tooltip.Root
  onOpenChange={(open, eventDetails) => {
    if (eventDetails.reason === 'trigger-press') {
      eventDetails.cancel(); // Prevent state change
    }
  }}
>
```

Prevent Base UI from handling specific events:

```tsx
<NumberField.Input
	onPaste={(event) => {
		event.preventBaseUIHandler();
	}}
/>
```

### React Hook Form integration

```tsx
import { useForm, Controller } from 'react-hook-form';
import { Field } from '@base-ui/react/field';
import { Select } from '@base-ui/react/select';

const { control, handleSubmit } = useForm({
	defaultValues: { serverType: '' }
});

<Controller
	name="serverType"
	control={control}
	rules={{ required: 'Required' }}
	render={({ field: { ref, value, onChange, onBlur }, fieldState }) => (
		<Field.Root name="serverType" invalid={fieldState.invalid}>
			<Field.Label>Server Type</Field.Label>
			<Select.Root value={value} onValueChange={onChange} inputRef={ref}>
				<Select.Trigger onBlur={onBlur}>
					<Select.Value />
				</Select.Trigger>
				<Select.Portal>
					<Select.Positioner>
						<Select.Popup>
							<Select.Item value="small">Small</Select.Item>
							<Select.Item value="medium">Medium</Select.Item>
						</Select.Popup>
					</Select.Positioner>
				</Select.Portal>
			</Select.Root>
			<Field.Error>{fieldState.error?.message}</Field.Error>
		</Field.Root>
	)}
/>;
```

**Note**: Use `inputRef` prop (not `ref`) on Select, Switch, Checkbox, and Slider.Thumb for form library focus management.

### Built-in form handling

```tsx
import { Field } from '@base-ui/react/field';
import { Form } from '@base-ui/react/form';

<Form
	onFormSubmit={async (values) => {
		await submitToServer(values);
	}}
>
	<Field.Root
		name="username"
		validationMode="onChange"
		validationDebounceTime={300}
		validate={async (value) => {
			if (value === 'admin') return 'Reserved username';
			return null;
		}}
	>
		<Field.Label>Username</Field.Label>
		<Field.Control required minLength={3} />
		<Field.Description>Choose a unique username</Field.Description>
		<Field.Error />
	</Field.Root>
</Form>;
```

## Accessibility implementation

### Automatic features

Base UI handles:

- ARIA roles and attributes (`role`, `aria-modal`, `aria-labelledby`, `aria-describedby`, `aria-expanded`, `aria-haspopup`)
- Keyboard navigation (Tab, Arrow keys, Home, End, Enter, Space, Escape)
- Focus management (trapping, restoration, roving tabindex)
- Screen reader announcements

### Developer responsibilities

1. **Accessible names** — provide labels for all form controls:

```tsx
// Using Field.Label (recommended)
<Field.Root>
  <Field.Label>Email</Field.Label>
  <Input />
</Field.Root>

// Using aria-label
<Input aria-label="Email address" />

// Icon buttons require aria-label
<Dialog.Trigger aria-label="Close">
  <CloseIcon />
</Dialog.Trigger>
```

2. **Focus indicators** — style focus states:

```css
button:focus-visible {
	outline: 2px solid #2563eb;
	outline-offset: 2px;
}
```

3. **Color contrast** — implement WCAG-compliant contrast ratios in styling.

### Focus management props

```tsx
// Custom initial focus
<Dialog.Popup initialFocus={firstInputRef}>

// Custom focus on close
<Dialog.Popup finalFocus={triggerRef}>

// Function-based (keyboard vs pointer)
<Dialog.Popup
  initialFocus={(interactionType) => {
    if (interactionType === 'keyboard') return inputRef.current;
    return true; // default behavior
  }}
/>

// Modal modes
<Dialog.Root modal={true}>    {/* Focus trapped, scroll locked */}
<Dialog.Root modal={false}>   {/* No trapping */}
<Dialog.Root modal="trap-focus"> {/* Focus trapped only, no scroll lock */}
```

## TypeScript patterns

### Namespaced types

```tsx
import { Tooltip } from '@base-ui/react/tooltip';
import { Popover } from '@base-ui/react/popover';

// Props type
function MyTooltip(props: Tooltip.Root.Props) {
	return <Tooltip.Root {...props} />;
}

// State type
function CustomPositioner(props: Popover.Positioner.Props, state: Popover.Positioner.State) {
	return (
		<div {...props}>
			<span>Side: {state.side}</span>
			{props.children}
		</div>
	);
}

// Event types
function handleChange(value: string, details: Combobox.Root.ChangeEventDetails) {}
```

### useRender for custom components

```tsx
import { useRender } from '@base-ui/react/use-render';
import { mergeProps } from '@base-ui/react/merge-props';

interface ButtonProps extends useRender.ComponentProps<'button'> {}

function Button({ render, ...props }: ButtonProps) {
	const defaultProps: useRender.ElementProps<'button'> = {
		className: 'btn',
		type: 'button'
	};

	return useRender({
		defaultTagName: 'button',
		render,
		props: mergeProps<'button'>(defaultProps, props)
	});
}
```

### useRender with state generic

```tsx
interface CounterState {
	count: number;
	isEven: boolean;
}

interface CounterProps extends useRender.ComponentProps<'button', CounterState> {}

function Counter({ render, ...props }: CounterProps) {
	const [count, setCount] = useState(0);
	const state = useMemo(() => ({ count, isEven: count % 2 === 0 }), [count]);

	return useRender({
		defaultTagName: 'button',
		render,
		state,
		props
	});
}

// Usage
<Counter
	render={(props, state) => (
		<button {...props}>
			Count: {state.count} ({state.isEven ? 'even' : 'odd'})
		</button>
	)}
/>;
```

## Performance optimization

### Bundle size

Use path-based imports for smaller bundles and faster development builds:

```tsx
// Recommended
import { Tooltip } from '@base-ui/react/tooltip';
import { Menu } from '@base-ui/react/menu';

// Avoid barrel imports in development
import { Tooltip, Menu } from '@base-ui/react';
```

### Memoization

Define slot components outside render functions:

```tsx
// Bad - component recreated each render
function App() {
	const [name, setName] = useState('');
	const CustomHeader = () => <input value={name} onChange={(e) => setName(e.target.value)} />;
	return <Component slots={{ header: CustomHeader }} />;
}

// Good - stable reference
const CustomHeader = ({ name, setName }) => (
	<input value={name} onChange={(e) => setName(e.target.value)} />
);

function App() {
	const [name, setName] = useState('');
	return <Component slots={{ header: CustomHeader }} slotProps={{ header: { name, setName } }} />;
}
```

For expensive render prop components, use `React.memo`:

```tsx
const ExpensiveThumb = React.memo(
	React.forwardRef<HTMLSpanElement, Props>((props, ref) => (
		<span ref={ref} {...props}>
			{/* expensive rendering */}
		</span>
	))
);

<Switch.Thumb render={<ExpensiveThumb />} />;
```

## Common mistakes

### Portal z-index issues

Popups appearing behind page content. Add stacking context isolation:

```tsx
// layout.tsx
<body>
  <div className="root">{children}</div>
</body>

// styles.css
.root {
  isolation: isolate;
}
```

### iOS Safari backdrop bug (iOS 26+)

Backdrops not covering viewport after scrolling. Add to global styles:

```css
body {
	position: relative;
}
```

### Missing ref/prop forwarding

Custom components in `render` prop fail silently:

```tsx
// Broken - no ref forwarding, props not spread
const BadButton = ({ onClick, children }) => <button onClick={onClick}>{children}</button>;

// Correct
const GoodButton = React.forwardRef<HTMLButtonElement, Props>((props, ref) => (
	<button ref={ref} {...props} />
));
```

### Event propagation in portaled content

Menu/popover clicks bubble to parent handlers:

```tsx
// Problem
<div onClick={handleParentClick}>
  <Menu.Root>
    <Menu.Popup>
      <Menu.Item>Triggers parent!</Menu.Item>
    </Menu.Popup>
  </Menu.Root>
</div>

// Solution
<Menu.Popup onClick={(e) => e.stopPropagation()}>
```

### Incorrect package

Using deprecated `@mui/base` instead of `@base-ui/react`. The old package uses different APIs (`slots`/`slotProps`) incompatible with current documentation.

### Missing accessible names

```tsx
// Missing label
<Input />

// With label
<Field.Root>
  <Field.Label>Email</Field.Label>
  <Input />
</Field.Root>
```

### Hidden focus indicators

```css
/* Removes accessibility */
button:focus {
	outline: none;
}

/* Visible focus indicator */
button:focus-visible {
	outline: 2px solid #2563eb;
	outline-offset: 2px;
}
```

## Available components

Accordion, Alert Dialog, Autocomplete, Avatar, Button, Checkbox, Checkbox Group, Collapsible, Combobox, Context Menu, Dialog, Field, Fieldset, Form, Input, Menu, Menubar, Meter, Navigation Menu, Number Field, Popover, Preview Card, Progress, Radio, Scroll Area, Select, Separator, Slider, Switch, Tabs, Toast, Toggle, Toggle Group, Toolbar, Tooltip.
