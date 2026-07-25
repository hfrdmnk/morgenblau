import { BookmarkIcon, SpinnerIcon } from '@proicons/react';
import { Fragment, useEffect, useState } from 'react';

import {
    ListPanelShell,
    SectionState,
} from '@/components/library/library-panel-shell';
import {
    RowDivider,
    RowOverlayLink,
    ROW_CLASS,
} from '@/components/library/share-row';
import { formatDate } from '@/lib/date';
import { fetchSaves, type Save } from '@/lib/library';
import {
    readSavedCache,
    writeCachedSaves,
    writeSavedCache,
} from '@/lib/library-cache';
import { shareTargetPresentation } from '@/lib/share-target';

type State =
    | { kind: 'loading' }
    | { kind: 'ok'; saves: Save[] }
    | { kind: 'error' };

// Stable empty list so list navigation doesn't reset every render while loading.
const EMPTY_SAVES: Save[] = [];

// SavedPanel: the Library "Saved" tab — everything this reader has kept for later.
export function SavedPanel() {
    const [state, setState] = useState<State>(() => {
        const cached = readSavedCache();
        return cached
            ? { kind: 'ok', saves: cached.saves }
            : { kind: 'loading' };
    });

    useEffect(() => {
        if (readSavedCache()) return;
        let cancelled = false;
        const load = async () => {
            try {
                const saves = await fetchSaves();
                if (cancelled) return;
                setState({ kind: 'ok', saves });
                writeSavedCache(saves);
            } catch {
                if (!cancelled) setState({ kind: 'error' });
            }
        };
        load();
        return () => {
            cancelled = true;
        };
    }, []);

    // Write-through keeps the cache in sync with in-place list edits without owning state itself.
    useEffect(() => {
        if (state.kind === 'ok') writeCachedSaves(state.saves);
    }, [state]);

    const items = state.kind === 'ok' ? state.saves : EMPTY_SAVES;

    return (
        <ListPanelShell eyebrow="Library" heading="Saved" items={items}>
            {(nav) => <Saves state={state} onActivate={nav.setActive} />}
        </ListPanelShell>
    );
}

function Saves({
    state,
    onActivate,
}: {
    state: State;
    onActivate: (index: number) => void;
}) {
    if (state.kind === 'loading') {
        return <SectionState icon={SpinnerIcon} spin lead="Loading…" />;
    }
    if (state.kind === 'error') {
        return (
            <SectionState
                lead="Couldn't load your saves."
                detail="Try again in a moment."
            />
        );
    }
    if (state.saves.length === 0) {
        return (
            <SectionState
                icon={BookmarkIcon}
                lead="Nothing saved yet."
                detail="Press B in the reader to keep an article for later."
            />
        );
    }

    return (
        <ul className="flex flex-col">
            {state.saves.map((save, index) => (
                <Fragment key={save.rkey}>
                    {index > 0 ? <RowDivider /> : null}
                    <SaveRow
                        save={save}
                        index={index}
                        onActivate={onActivate}
                    />
                </Fragment>
            ))}
        </ul>
    );
}

function SaveRow({
    save,
    index,
    onActivate,
}: {
    save: Save;
    index: number;
    onActivate: (index: number) => void;
}) {
    const target = shareTargetPresentation(save);

    return (
        <li
            data-nav-row=""
            onMouseEnter={() => onActivate(index)}
            className={ROW_CLASS}
        >
            <RowOverlayLink target={target} />
            <div className="pointer-events-none min-w-0 flex-1">
                <h3 className="line-clamp-1 text-heading text-foreground">
                    {target.label}
                </h3>
                <p className="mt-1 text-caption text-muted-foreground">
                    {formatDate(save.createdAt)}
                </p>
            </div>
        </li>
    );
}
