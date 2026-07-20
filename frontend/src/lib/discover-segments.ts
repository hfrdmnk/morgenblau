import type { DiscoverSource } from '@/lib/discover';

export type SourceSegment<T extends { key: string } = DiscoverSource> = {
    key: string;
    expanded: boolean;
    masthead: boolean;
    sources: T[];
};

// The masthead seeds the top run so it's always segment 0 keyed 'masthead', emptying out once
// its run's head expands. Other collapsed neighbors coalesce; each expanded source stands alone.
export function partitionSourceSegments<T extends { key: string }>(
    sources: T[],
    expandedKeys: ReadonlySet<string>,
): SourceSegment<T>[] {
    if (sources.length === 0) return [];

    const segments: SourceSegment<T>[] = [];
    const masthead: SourceSegment<T> = { key: 'masthead', expanded: false, masthead: true, sources: [] };
    segments.push(masthead);
    let run: SourceSegment<T> | null = masthead;
    for (const source of sources) {
        if (expandedKeys.has(source.key)) {
            run = null;
            segments.push({ key: source.key, expanded: true, masthead: false, sources: [source] });
            continue;
        }
        if (!run) {
            run = { key: source.key, expanded: false, masthead: false, sources: [] };
            segments.push(run);
        }
        run.sources.push(source);
    }
    return segments;
}
