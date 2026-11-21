import { expect, test } from 'bun:test';
import { renderToStaticMarkup } from 'react-dom/server';

import { Frame } from '@/design/frame';

const classesOf = (element: React.ReactElement) =>
    (renderToStaticMarkup(element).match(/class="([^"]*)"/)?.[1] ?? '').split(' ').sort();

test('a Frame stays a flex column below the first breakpoint a responsive display or direction names', () => {
    expect(classesOf(<Frame />)).toEqual(['flex', 'flex-col']);
    expect(classesOf(<Frame display={{ md: 'none' }} direction={{ sm: 'row' }} />)).toEqual(
        ['flex', 'flex-col', 'md:hidden', 'sm:flex-row'].sort(),
    );
    expect(classesOf(<Frame direction={{ base: undefined, lg: 'row' }} />)).toEqual([
        'flex',
        'flex-col',
        'lg:flex-row',
    ]);
    expect(classesOf(<Frame display={{ base: 'grid', sm: 'flex' }} direction="row" />)).toEqual(
        ['flex-row', 'grid', 'sm:flex'].sort(),
    );
});
