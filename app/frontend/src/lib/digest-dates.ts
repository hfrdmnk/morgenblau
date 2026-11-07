const DATE_KEY = /^(\d{4})-(\d{2})-(\d{2})$/;

export function toDateKey(date: Date): string {
    const year = date.getFullYear();
    const month = String(date.getMonth() + 1).padStart(2, '0');
    const day = String(date.getDate()).padStart(2, '0');
    return `${year}-${month}-${day}`;
}

export function parseDigestDate(value: string | null): Date | null {
    if (!value) return null;
    const match = DATE_KEY.exec(value);
    if (!match) return null;

    const year = Number(match[1]);
    const month = Number(match[2]);
    const day = Number(match[3]);
    const date = new Date(year, month - 1, day);

    return toDateKey(date) === value ? date : null;
}

export function addCalendarDays(date: Date, amount: number): Date {
    const result = new Date(date);
    result.setDate(result.getDate() + amount);
    return result;
}

export function digestDateRange(selected: Date, today: Date): Date[] {
    const daysAfter = Math.min(
        3,
        Math.max(0, Math.round((startOfDay(today).getTime() - startOfDay(selected).getTime()) / 86_400_000)),
    );
    const dates: Date[] = [];
    for (let offset = -3; offset <= daysAfter; offset += 1) {
        dates.push(addCalendarDays(selected, offset));
    }
    return dates;
}

export function startOfToday(): Date {
    return startOfDay(new Date());
}

export function selectedDigestDate(search: string, today: Date): Date {
    const requested = parseDigestDate(new URLSearchParams(search).get('date'));
    return requested && requested <= today ? requested : today;
}

const WEEKDAYS = ['sunday', 'monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday'];
const MONTHS = ['january', 'february', 'march', 'april', 'may', 'june', 'july', 'august', 'september', 'october', 'november', 'december'];

// Three letters at least, so a short query like "mo" stays a search instead of becoming a date.
function namedIndex(names: string[], word: string): number {
    return word.length < 3 ? -1 : names.findIndex((name) => name.startsWith(word));
}

function calendarDate(year: number, month: number, day: number): Date | null {
    const date = new Date(year, month, day);
    return date.getMonth() === month && date.getDate() === day ? date : null;
}

// A day without a year is the latest one that is not in the future.
function latestOn(month: number, day: number, year: number | undefined, today: Date): Date | null {
    if (year !== undefined) return calendarDate(year, month, day);
    const thisYear = calendarDate(today.getFullYear(), month, day);
    return thisYear && thisYear <= today ? thisYear : calendarDate(today.getFullYear() - 1, month, day);
}

function fullYear(value: string | undefined): number | undefined {
    if (!value) return undefined;
    return value.length === 2 ? 2000 + Number(value) : Number(value);
}

function relativeDay(query: string, today: Date): Date | null {
    if (namedIndex(['today'], query) === 0) return today;
    if (namedIndex(['yesterday'], query) === 0) return addCalendarDays(today, -1);
    if (query === 'last week') return addCalendarDays(today, -7);
    const ago = /^(\d{1,4}|a|one)\s*(d|days?|w|weeks?)(?: ago)?$/.exec(query);
    if (ago) {
        const amount = /^\d/.test(ago[1]) ? Number(ago[1]) : 1;
        return addCalendarDays(today, -amount * (ago[2].startsWith('w') ? 7 : 1));
    }
    // A weekday is the latest one before today, so "monday" on a Monday means a week ago.
    const weekday = namedIndex(WEEKDAYS, query.replace(/^last /, ''));
    if (weekday < 0) return null;
    return addCalendarDays(today, -(((today.getDay() - weekday + 6) % 7) + 1));
}

function writtenDay(query: string, today: Date): Date | null {
    const iso = parseDigestDate(query);
    if (iso) return iso;
    const dotted = /^(\d{1,2})\.(\d{1,2})\.?(\d{4}|\d{2})?$/.exec(query);
    if (dotted) return latestOn(Number(dotted[2]) - 1, Number(dotted[1]), fullYear(dotted[3]), today);
    const dayFirst = /^(\d{1,2})\.? ([a-z]+),? ?(\d{4})?$/.exec(query);
    const monthFirst = /^([a-z]+) (\d{1,2}),? ?(\d{4})?$/.exec(query);
    const [day, name, year] = dayFirst
        ? [dayFirst[1], dayFirst[2], dayFirst[3]]
        : monthFirst ? [monthFirst[2], monthFirst[1], monthFirst[3]] : [];
    const month = name ? namedIndex(MONTHS, name) : -1;
    if (month < 0) return null;
    return latestOn(month, Number(day), fullYear(year), today);
}

// Reads a typed day such as "yesterday", "last monday", "2 days ago", "14.10" or "oct 14". Future days have no digest, so they read as nothing.
export function parseDayQuery(input: string, today: Date): Date | null {
    const query = input.trim().toLowerCase().replace(/\s+/g, ' ');
    if (!query) return null;
    const day = relativeDay(query, today) ?? writtenDay(query, today);
    return day && day <= today ? day : null;
}

function startOfDay(date: Date): Date {
    return new Date(date.getFullYear(), date.getMonth(), date.getDate());
}
