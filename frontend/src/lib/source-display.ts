type Frequency = 'daily' | 'weekly' | 'biweekly' | 'monthly' | 'new' | 'irregular' | 'noPosts';

const FREQUENCY_LABELS: Record<Frequency, string> = {
    daily: 'Daily',
    weekly: 'Weekly',
    biweekly: 'Every two weeks',
    monthly: 'Monthly',
    new: 'New',
    irregular: 'Irregular',
    noPosts: 'Nothing yet',
};

function frequencyLabel(frequency?: string): string | undefined {
    return FREQUENCY_LABELS[frequency as Frequency];
}

export function displayURL(value: string): string {
    return value.replace(/^https?:\/\//, '').replace(/\/$/, '');
}

const longDate = new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'long', year: 'numeric' });

export function formatDate(value?: string | null): string | undefined {
    if (!value) return undefined;
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? undefined : longDate.format(date);
}

function countLabel(count: number, unit: string): string | undefined {
    if (count <= 0) return undefined;
    return `${count} ${count === 1 ? unit : `${unit}s`}`;
}

function facts(label: string | undefined, frequency: string | undefined, count: number, unit: string, last?: string | null): string[] {
    const lastDate = formatDate(last);
    return [label, frequencyLabel(frequency), countLabel(count, unit), lastDate && `Last ${unit} ${lastDate}`]
        .filter((fact): fact is string => Boolean(fact));
}

export function feedFacts(source: { primary: boolean; frequency?: string; totalEntries: number; lastPublishedAt?: string }): string[] {
    return facts(source.primary ? 'Primary' : undefined, source.frequency, source.totalEntries, 'post', source.lastPublishedAt);
}

// A stopped newsletter has no cadence to promise and its primary flag no longer applies.
export function newsletterFacts(source: { primary: boolean; status: string; frequency: string; issueCount: number; lastReceivedAt: string | null }): string[] {
    if (source.status === 'stopped') return facts('Stopped', undefined, source.issueCount, 'issue', source.lastReceivedAt);
    return facts(source.primary ? 'Primary' : undefined, source.frequency, source.issueCount, 'issue', source.lastReceivedAt);
}
