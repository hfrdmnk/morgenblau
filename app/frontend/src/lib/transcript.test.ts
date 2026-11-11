import { expect, test } from 'bun:test';

import { activeIndex, formatTimestamp, groupCues, type TranscriptCue } from './transcript';

const cue = (start: number, text: string, duration = 3000): TranscriptCue => ({ start, duration, text });

test('a sentence ends a paragraph only once the paragraph has run a while', () => {
    const paragraphs = groupCues([
        cue(0, 'Short opener.'),
        cue(3000, 'Still the first paragraph,'),
        cue(6000, 'and it keeps going.'),
        cue(13000, 'A sentence after twelve seconds.'),
        cue(16000, 'This starts the second one.'),
    ]);
    expect(paragraphs).toEqual([
        { start: 0, text: 'Short opener. Still the first paragraph, and it keeps going. A sentence after twelve seconds.' },
        { start: 16000, text: 'This starts the second one.' },
    ]);
});

test('a long pause or the cap breaks a paragraph without punctuation', () => {
    const paragraphs = groupCues([
        cue(0, 'auto captions'),
        cue(3000, 'have no punctuation', 2000),
        cue(26000, 'after a pause'),
        cue(40000, 'still going'),
        cue(56000, 'past thirty seconds'),
    ]);
    expect(paragraphs.map((p) => p.start)).toEqual([0, 26000, 56000]);
});

test('the active paragraph is the last one that started', () => {
    const paragraphs = [{ start: 4000, text: 'a' }, { start: 10000, text: 'b' }, { start: 60000, text: 'c' }];
    expect(activeIndex(paragraphs, 3999)).toBe(-1);
    expect(activeIndex(paragraphs, 4000)).toBe(0);
    expect(activeIndex(paragraphs, 9999)).toBe(0);
    expect(activeIndex(paragraphs, 10000)).toBe(1);
    expect(activeIndex(paragraphs, 45000)).toBe(1);
    expect(activeIndex(paragraphs, 10_000_000)).toBe(2);
    expect(activeIndex([], 5000)).toBe(-1);
});

test('timestamps read like YouTube', () => {
    expect(formatTimestamp(0)).toBe('0:00');
    expect(formatTimestamp(65_999)).toBe('1:05');
    expect(formatTimestamp(3_723_000)).toBe('1:02:03');
});
