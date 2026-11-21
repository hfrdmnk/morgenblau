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
