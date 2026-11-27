import { RuleTester } from 'eslint';

import designSystem from './design-system.js';

const tester = new RuleTester({
    languageOptions: { parserOptions: { ecmaFeatures: { jsx: true } } },
});

tester.run('no-raw-elements', designSystem.rules['no-raw-elements'], {
    valid: [
        '<Frame as="nav" gap="m"><Text as="h2">Title</Text></Frame>',
        '<MotionFrame layout />',
        '<Dialog.Title />',
        '<svg><path d="M0 0" /></svg>',
    ],
    invalid: [
        { code: '<div />', errors: [{ message: /Use <Frame> instead of <div>/ }] },
        { code: '<section />', errors: [{ message: /Use <Frame as="section"> instead/ }] },
        { code: '<p />', errors: [{ message: /Use <Text> instead of <p>/ }] },
        { code: '<h2 />', errors: [{ message: /Use <Text as="h2"> instead/ }] },
        { code: '<button />', errors: [{ message: /<Button> from @\/components\/ui\/button/ }] },
        { code: '<a href="/" />', errors: [{ message: /<Link> from @\/components\/ui\/link/ }] },
        { code: '<motion.div />', errors: [{ message: /<MotionFrame> .* instead of <motion\.div>/ }] },
    ],
});

tester.run('no-style-props', designSystem.rules['no-style-props'], {
    valid: ['<Frame gap="m" data-post aria-label="Posts" />', '<Button variant="ghost" />'],
    invalid: [
        { code: '<Frame className="p-4" />', errors: 1 },
        { code: '<Button style={{ color: "red" }} />', errors: 1 },
        { code: '<Icon className={cn("a")} style={s} />', errors: 2 },
    ],
});

tester.run('no-raw-elements outside JSX', designSystem.rules['no-raw-elements'], {
    valid: [
        "document.createElement('form')",
        "import { createElement } from 'react'; createElement(as ?? 'div')",
        "import { createElement } from 'react'; createElement(Frame, {})",
    ],
    invalid: [
        { code: "import { motion as m } from 'motion/react'; <m.div />", errors: [{ message: /<MotionFrame>/ }] },
        {
            code: "import * as Motion from 'motion/react'; <Motion.motion.span />",
            errors: [{ message: /<MotionFrame>/ }],
        },
        { code: "import { motion } from 'motion/react'; motion.create('div')", errors: [{ message: /<MotionFrame>/ }] },
        { code: "import { motion } from 'motion/react'; motion('section')", errors: [{ message: /<MotionFrame>/ }] },
        {
            code: "import { createElement } from 'react'; createElement('div')",
            errors: [{ message: /Use <Frame> instead of createElement\('div'\)/ }],
        },
        { code: "import React from 'react'; React.createElement('a', {})", errors: [{ message: /<Link>/ }] },
    ],
});

tester.run('no-style-props in objects', designSystem.rules['no-style-props'], {
    valid: ["const props = { 'aria-label': 'Posts', gap: 'm' }; <Frame {...props} />", 'const cls = { [key]: 1 }'],
    invalid: [
        { code: "const p = { className: 'p-4' }; <X {...p} />", errors: 1 },
        { code: "<X {...{ style: { color: 'red' } }} />", errors: 1 },
        { code: "cloneElement(child, { 'className': 'p-4' })", errors: 1 },
    ],
});

const classes = (...list: string[]) => list.map((value) => `const c = ${JSON.stringify(value)}`);
const flagged = (value: string, message: RegExp) => ({
    code: `const c = ${JSON.stringify(value)}`,
    errors: [{ message }],
});

tester.run(
    'token-classes reads every string as a class list: class names reach components through variables, cva maps and helpers, not only className',
    designSystem.rules['token-classes'],
    {
        valid: [
            ...classes(
                'flex gap-2 bg-primary text-sm/6 text-2xl/7 hover:bg-foreground-faint',
                'data-[side=top]:-bottom-1 has-[[data-slot=input]:disabled]:opacity-45 group-data-[size=sm]/card:text-sm',
                "[&_svg:not([class*='size-'])]:size-4 [&_[cmdk-group-heading]]:px-3 transition-[opacity,transform]",
                'top-(--palette-top) max-h-(--dialog-max-height) bg-linear-to-b/oklab w-1/2 -translate-x-1/2 aspect-4/3',
                '[data-slot=\'input\'] [role="dialog"][data-open]',
                'https://example.com/a?b=1 Switch to dark mode',
            ),
            { code: 'const c = "md:dark:bg-primary hidden dark:block"', options: [{ allowDark: true }] },
        ],
        invalid: [
            flagged('dark:bg-primary', /^dark:bg-primary: .*dark: variant/),
            flagged('md:dark:bg-primary', /dark: variant/),
            flagged('bg-red-500', /raw palette color/),
            flagged('shadow-white', /raw palette color/),
            flagged('ring-offset-white', /raw palette color/),
            flagged('accent-red-500', /raw palette color/),
            flagged('btn-surface-red-500', /raw palette color/),
            flagged('border-t-stone-200!', /raw palette color/),
            flagged('bg-primary/50', /opacity modifier/),
            flagged('bg-primary/(--x)', /opacity modifier/),
            flagged('shadow-black/50', /raw palette color/),
            flagged('caret-primary/50', /opacity modifier/),
            flagged('hover:text-foreground/75', /opacity modifier/),
            flagged('bg-primary/[0.5]', /arbitrary value/),
            flagged('text-sm/[1.2]', /arbitrary value/),
            flagged('w-[3px]', /arbitrary value/),
            flagged('-mt-[3px]', /arbitrary value/),
            flagged('!w-[3px]', /arbitrary value/),
            flagged('content-["[x]"]', /arbitrary value/),
            flagged('grid-cols-[1fr_auto]', /arbitrary value/),
            flagged('max-[600px]:hidden', /arbitrary value/),
            flagged('min-[30rem]:flex', /arbitrary value/),
            flagged('@min-[400px]:flex', /arbitrary value/),
            flagged('[--btn-surface:var(--primary)]', /arbitrary property/),
            flagged('![color:red]', /arbitrary property/),
            flagged('data-[ending-style]:[transform:translateY(100%)]', /arbitrary property/),
            { code: 'const c = `${base} w-[3px] bg-red-500`', errors: 2 },
            {
                code: 'const c = "md:dark:bg-red-500"',
                options: [{ allowDark: true }],
                errors: [{ message: /raw palette color/ }],
            },
        ],
    },
);
