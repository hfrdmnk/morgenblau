// Case-insensitive union across any number of tag lists; first-seen casing wins, result sorted.
export function mergeTagSuggestions(...tagLists: string[][]): string[] {
    const byLower = new Map<string, string>();
    for (const tags of tagLists) {
        for (const tag of tags) {
            const key = tag.toLowerCase();
            if (!byLower.has(key)) byLower.set(key, tag);
        }
    }
    return [...byLower.values()].sort((a, b) => a.localeCompare(b));
}
