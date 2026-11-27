import type { ComponentProps, ReactNode } from 'react';

import '@/pages/reader.css';

function TranscriptList({
    font,
    stuck,
    ...props
}: Omit<ComponentProps<'ol'>, 'className' | 'style'> & { font: string; stuck: boolean }) {
    return (
        <ol
            data-slot="transcript"
            className="reader-transcript"
            data-font={font}
            data-stuck={stuck ? '' : undefined}
            {...props}
        />
    );
}

function TranscriptPassage({
    time,
    label,
    current,
    onPlay,
    children,
}: {
    time: string;
    label: string;
    current: boolean;
    onPlay?: () => void;
    children: ReactNode;
}) {
    return (
        <li aria-current={current ? 'true' : undefined} data-post={onPlay ? '' : undefined}>
            {onPlay ? (
                <button aria-label={label} data-post-link onClick={onPlay} type="button">
                    {time}
                </button>
            ) : (
                <span>{time}</span>
            )}
            <p>{children}</p>
        </li>
    );
}

export { TranscriptList, TranscriptPassage };
