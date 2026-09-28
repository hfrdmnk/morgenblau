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

function startOfDay(date: Date): Date {
    return new Date(date.getFullYear(), date.getMonth(), date.getDate());
}
