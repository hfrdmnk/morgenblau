type Slot = 'morning' | 'afternoon' | 'evening';

const GREETINGS: Record<Slot, string[]> = {
    morning: ['Good morning', 'Rise and shine', 'A fresh start', 'Morning'],
    afternoon: ['Good afternoon', 'Hello again', 'A quiet afternoon'],
    evening: ['Good evening', 'Winding down', 'A calm evening'],
};

function slotForHour(hour: number): Slot {
    if (hour < 12) return 'morning';
    if (hour < 18) return 'afternoon';
    return 'evening';
}

// Picks a time-appropriate greeting at random; appends the name when present.
export function pickGreeting(name: string | null, now: Date = new Date()): string {
    const list = GREETINGS[slotForHour(now.getHours())];
    const phrase = list[Math.floor(Math.random() * list.length)];
    return name ? `${phrase}, ${name}` : phrase;
}
