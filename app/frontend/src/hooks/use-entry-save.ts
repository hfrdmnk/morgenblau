import { useState, useSyncExternalStore } from 'react';

import { createEntrySave, type EntrySave } from '@/lib/entry-save';
import type { ReaderEntry } from '@/lib/reader';

export function useEntrySave(entry: ReaderEntry): EntrySave | null {
    const [save] = useState(() => createEntrySave(entry));
    return save;
}

export function useSaveState(save: EntrySave) {
    return useSyncExternalStore(save.subscribe, save.get);
}
