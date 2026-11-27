import { RuleTester } from 'eslint';

import comments from './comments.js';

const tester = new RuleTester({
    languageOptions: { parserOptions: { ecmaFeatures: { jsx: true } } },
});

const policy = /^Comments go stale and nothing enforces them\./;

tester.run('no-comments', comments.rules['no-comments'], {
    valid: [
        'const x = 1;',
        '// SQLite rejects this, see\n// https://docs.example.com/issue/1\nconst x = 1;',
        '/* see https://docs.example.com/issue/1 */\nconst x = 1;',
        '// Copyright 2026 Example Author\n// SPDX-License-Identifier: MIT\nconst x = 1;',
        '/// <reference types="vite/client" />\nconst x = 1;',
        '// prettier-ignore\nconst x = [1,0,\n0,1];',
        '#!/usr/bin/env node\nconst x = 1;',
        '// eslint-disable-next-line no-console\nconsole.log(1);',
        'const el = <div>{/* see https://docs.example.com/issue/1 */}</div>;',
    ],
    invalid: [
        {
            code: '// one\n// two\nconst x = 1; /* three\nfour */',
            errors: [{ message: policy }, { message: policy, line: 2 }, { message: policy, line: 3 }],
        },
        { code: '/** Does things. */\nexport function f() {}', errors: 1 },
        { code: 'const el = <div>{/* note */}</div>;', errors: 1 },
        {
            code: '// see https://docs.example.com/issue/1\n\n// unrelated\nconst x = 1;',
            errors: [{ message: policy, line: 3 }],
        },
        {
            code: 'const x = 1; // trailing\n// https://docs.example.com/issue/1\nconst y = 2;',
            errors: [{ message: policy, line: 1 }],
        },
        {
            code: '// https://app.morgen.blue/about\n// https://github.com/hfrdmnk/morgenblau/issues/1\n// https://tangled.org/@owner.example/morgenblau\nconst x = 1;',
            errors: 3,
        },
        { code: 'const x = 1;\n// Copyright notice', errors: 1 },
    ],
});

const noInlineConfigTester = new RuleTester({ linterOptions: { noInlineConfig: true } });

const disabled = [
    { message: /has no effect because you have 'noInlineConfig'/ },
    { message: /^Fix the code instead of disabling the rule.*Comments go stale/ },
];

noInlineConfigTester.run('no-eslint-disable', comments.rules['no-eslint-disable'], {
    valid: ['// https://docs.example.com/eslint\nconst x = 1;'],
    invalid: [
        { code: '// eslint-disable-next-line no-console\nconsole.log(1);', errors: disabled },
        { code: '/* eslint-disable */\nconst x = 1;', errors: disabled },
        { code: 'const x = 1; // eslint-disable-line', errors: disabled },
    ],
});
