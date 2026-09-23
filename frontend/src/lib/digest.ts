import { formatISODate } from '@/lib/date';

export function digestRequestPath(date: Date, timezone?: string): string {
    const localTimezone =
        timezone || Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
    return `/api/digest?date=${encodeURIComponent(formatISODate(date))}&timezone=${encodeURIComponent(localTimezone)}`;
}
