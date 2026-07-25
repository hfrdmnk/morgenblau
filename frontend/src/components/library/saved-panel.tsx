import { BookmarkIcon } from '@proicons/react';

import { CardMasthead } from '@/components/card-masthead';
import {
    Divider,
    SectionState,
} from '@/components/library/library-panel-shell';

// No list endpoint exists yet; this tab stays the designed empty state until saves land.
export function SavedPanel() {
    return (
        <article className="overflow-hidden rounded-xl bg-card shadow-card">
            <CardMasthead eyebrow="Library" heading="Saved" />
            <Divider />
            <SectionState
                icon={BookmarkIcon}
                lead="Nothing to show yet."
                detail="Save an article from the reader. A list of your saves is coming soon."
            />
        </article>
    );
}
