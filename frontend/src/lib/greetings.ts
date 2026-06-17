import { startOfLocalDay } from '@/lib/date';

type Slot = 'morning' | 'afternoon' | 'evening';

const GREETINGS: Record<Slot, string[]> = {
    morning: ['Good morning', 'Rise and shine', 'A fresh start', 'Morning'],
    afternoon: ['Good afternoon', 'Hello again', 'A quiet afternoon'],
    evening: ['Good evening', 'Winding down', 'A calm evening'],
};

// Headings for past editions: no time-of-day, no name — a calmer archive tone.
const PAST_TITLES: string[] = [
    'Looking back',
    'From the archive',
    'A quiet revisit',
    "That morning's edition",
];

function slotForHour(hour: number): Slot {
    if (hour < 12) return 'morning';
    if (hour < 18) return 'afternoon';
    return 'evening';
}

// Per-day seed: stable across re-renders, consecutive across adjacent days.
function daySeed(date: Date): number {
    return Math.floor(startOfLocalDay(date).getTime() / 86_400_000);
}

function pickFrom(list: string[], date: Date): string {
    return list[daySeed(date) % list.length];
}

// Time-appropriate greeting for today's edition; appends the name when present.
export function pickGreeting(
    name: string | null,
    date: Date,
    now: Date = new Date(),
): string {
    const phrase = pickFrom(GREETINGS[slotForHour(now.getHours())], date);
    return name ? `${phrase}, ${name}` : phrase;
}

// Heading for a past edition, re-picked as you move between days.
export function pickPastTitle(date: Date): string {
    return pickFrom(PAST_TITLES, date);
}
