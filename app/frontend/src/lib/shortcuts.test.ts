import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, describe, expect, test } from 'bun:test';

GlobalRegistrator.register();
const { shortcutKey } = await import('./shortcuts');
afterAll(() => GlobalRegistrator.unregister());

function press(init: KeyboardEventInit, target: Element = document.body) {
    const event = new KeyboardEvent('keydown', { bubbles: true, ...init });
    Object.defineProperty(event, 'target', { value: target });
    return shortcutKey(event);
}

function element(html: string) {
    const host = document.createElement('div');
    host.innerHTML = html;
    document.body.append(host);
    return host.querySelector('[data-target]') as Element;
}

describe('shortcut keys', () => {
    test('Shift with a letter or arrow names the shortcut, whatever the letter case', () => {
        expect(press({ key: 'D', shiftKey: true })).toBe('Shift+D');
        expect(press({ key: 'd', shiftKey: true })).toBe('Shift+D');
        expect(press({ key: 'ArrowDown', shiftKey: true })).toBe('Shift+ArrowDown');
    });

    test('a key without a modifier keeps its plain name', () => {
        expect(press({ key: 'Escape' })).toBe('Escape');
        expect(press({ key: 'j' })).toBe('j');
        expect(press({ key: 'J' })).toBe('j');
        expect(press({ key: 'Escape', shiftKey: true })).toBe('Shift+Escape');
    });

    test('other modifiers and held keys are not shortcuts', () => {
        expect(press({ key: 'D', shiftKey: true, ctrlKey: true })).toBeNull();
        expect(press({ key: 'D', shiftKey: true, metaKey: true })).toBeNull();
        expect(press({ key: 'ArrowLeft', shiftKey: true, altKey: true })).toBeNull();
        expect(press({ key: 'ArrowDown', shiftKey: true, repeat: true })).toBeNull();
    });

    test('typing and open dialogs keep their keys', () => {
        for (const html of [
            '<input data-target>',
            '<textarea data-target></textarea>',
            '<select data-target></select>',
            '<div contenteditable="true"><p data-target>text</p></div>',
            '<div role="dialog"><button data-target>Add</button></div>',
        ]) {
            expect(press({ key: 'S', shiftKey: true }, element(html))).toBeNull();
            expect(press({ key: 'Escape' }, element(html))).toBeNull();
        }
        expect(press({ key: 'S', shiftKey: true }, element('<div contenteditable="false"><a data-target>link</a></div>'))).toBe('Shift+S');
    });
});
