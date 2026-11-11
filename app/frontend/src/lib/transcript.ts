export type TranscriptCue = { start: number; duration: number; text: string };

export type Transcript =
    | { status: 'none' }
    | { status: 'ready'; language: string; kind: 'manual' | 'asr'; cues: TranscriptCue[] };

export type TranscriptParagraph = { start: number; text: string };

// A paragraph ends at a sentence once it has run a while, at a long pause, or at a hard cap for captions without punctuation.
const SENTENCE_MS = 12_000;
const PAUSE_MS = 20_000;
const MAX_MS = 30_000;
const SENTENCE_END = /[.!?…]["'”’)\]]*$/;

export function groupCues(cues: TranscriptCue[]): TranscriptParagraph[] {
    const paragraphs: TranscriptParagraph[] = [];
    let current: { start: number; end: number; lines: string[] } | null = null;
    for (const cue of cues) {
        if (current) {
            const span = cue.start - current.start;
            const sentenceDone = SENTENCE_END.test(current.lines.at(-1) ?? '') && current.end - current.start >= SENTENCE_MS;
            if (sentenceDone || cue.start - current.end > PAUSE_MS || span >= MAX_MS) {
                paragraphs.push({ start: current.start, text: current.lines.join(' ') });
                current = null;
            }
        }
        current ??= { start: cue.start, end: cue.start, lines: [] };
        current.lines.push(cue.text);
        current.end = cue.start + cue.duration;
    }
    if (current) paragraphs.push({ start: current.start, text: current.lines.join(' ') });
    return paragraphs;
}

// The paragraph being spoken at ms: the last one that has started, so a pause keeps the previous one marked. -1 before the first.
export function activeIndex(paragraphs: TranscriptParagraph[], ms: number): number {
    let low = 0;
    let high = paragraphs.length - 1;
    let found = -1;
    while (low <= high) {
        const mid = (low + high) >> 1;
        if (paragraphs[mid].start <= ms) {
            found = mid;
            low = mid + 1;
        } else {
            high = mid - 1;
        }
    }
    return found;
}

export function formatTimestamp(ms: number): string {
    const total = Math.floor(ms / 1000);
    const hours = Math.floor(total / 3600);
    const minutes = Math.floor((total % 3600) / 60);
    const seconds = String(total % 60).padStart(2, '0');
    return hours ? `${hours}:${String(minutes).padStart(2, '0')}:${seconds}` : `${minutes}:${seconds}`;
}
