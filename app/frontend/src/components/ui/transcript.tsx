import type { ComponentProps, ReactNode } from 'react';

import '@/pages/reader.css';

// A video's transcript in the reader's typography (.reader-transcript in src/pages/reader.css); stuck tells it the player is pinned above.
function TranscriptList({ font, stuck, ...props }: Omit<ComponentProps<'ol'>, 'className' | 'style'> & { font: string; stuck: boolean }) {
    return <ol data-slot="transcript" className="reader-transcript" data-font={font} data-stuck={stuck ? '' : undefined} {...props} />;
}

// Until the player is ready a passage only shows its time; once it is, the time plays from there and the passage joins its PostList.
function TranscriptPassage({ time, label, current, onPlay, children }: {
    time: string;
    label: string;
    current: boolean;
    onPlay?: () => void;
    children: ReactNode;
}) {
    return (
        <li aria-current={current ? 'true' : undefined} data-post={onPlay ? '' : undefined}>
            {onPlay ? <button aria-label={label} data-post-link onClick={onPlay} type="button">{time}</button> : <span>{time}</span>}
            <p>{children}</p>
        </li>
    );
}

export { TranscriptList, TranscriptPassage };
