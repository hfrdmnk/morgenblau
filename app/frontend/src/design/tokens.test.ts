import { compile } from '@tailwindcss/node';
import { expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { breakpoints, frame, text } from '@/design/tokens';

// Every class a selector in the build names, unescaped; an exact set, since .p-1 is a substring of .p-1\.5.
const classNames = (css: string) =>
    new Set([...css.matchAll(/\.((?:\\.|[\w-])+)/g)].map(([, name]) => name.replace(/\\(.)/g, '$1')));

// Without a scanned candidate list the build holds only what index.css itself names, so a class missing here is one
// Frame or Text would put on an element with no CSS behind it.
test('every token class compiles at every breakpoint', async () => {
    const path = fileURLToPath(new URL('../index.css', import.meta.url));
    const compiler = await compile(readFileSync(path, 'utf8'), {
        base: fileURLToPath(new URL('..', import.meta.url)),
        onDependency() {},
    });
    const built = classNames(compiler.build([]));
    const classes = [frame, text].flatMap((maps) =>
        Object.values(maps).flatMap((map) =>
            Object.values(map as Record<string, string>).flatMap((value) => value.split(' ')),
        ),
    );
    const missing = classes
        .flatMap((cls) => [cls, ...breakpoints.map((breakpoint) => `${breakpoint}:${cls}`)])
        .filter((cls) => !built.has(cls));
    expect(missing, 'Run `bun run design:tokens`, or add the token’s value to src/index.css').toEqual([]);
});
