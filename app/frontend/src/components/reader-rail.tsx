import { useEffect, useState } from 'react';

import { BookmarkIcon, ExternalLinkIcon } from '@/components/icons';
import { announceRemove, announceSave, dismissSaveToasts } from '@/lib/save-announce';
import { useSaveState } from '@/hooks/use-entry-save';
import type { EntrySave } from '@/lib/entry-save';
import { httpURL, youtubeID, type ReaderEntry } from '@/lib/reader';
import { cn } from '@/lib/utils';

const RAIL_BUTTON =
    'inline-flex size-11 cursor-pointer items-center justify-center rounded-md sm:size-10 transition-[color,transform] duration-(--motion-duration-fast) ease-out motion-safe:active:scale-[0.96]';

export function ReaderRail({ entry, save }: { entry: ReaderEntry; save: EntrySave | null }) {
    const progress = useScrollProgress();
    useEffect(() => dismissSaveToasts, []);
    const actions = <RailActions entry={entry} save={save} />;

    return (
        <>
            <aside aria-label="Reader actions" className="pointer-events-none fixed top-1/2 right-4 z-10 hidden -translate-y-1/2 sm:right-6 sm:block">
                <div className="pointer-events-auto flex flex-col items-center gap-2">
                    {actions}
                    <ProgressRing progress={progress} />
                </div>
            </aside>
            <aside aria-label="Reader actions" className="fixed inset-x-0 bottom-0 z-10 border-t border-border bg-background/95 pb-[env(safe-area-inset-bottom)] backdrop-blur sm:hidden">
                <ProgressBar progress={progress} />
                <div className="flex items-center justify-around px-4 py-1">{actions}</div>
            </aside>
        </>
    );
}

function sourceActionLabel(entry: ReaderEntry): string {
    const labels: Record<string, string> = { newsletter: 'Open web version', video: 'Watch at source', microblog: 'View original post' };
    const isYoutube = entry.contentType === 'video' && youtubeID(entry.url);
    return isYoutube ? 'Watch on YouTube' : labels[entry.contentType] ?? 'Read original';
}

function RailActions({ entry, save }: { entry: ReaderEntry; save: EntrySave | null }) {
    const original = httpURL(entry.url);
    return (
        <>
            {save ? <SaveButton save={save} /> : null}
            {original ? (
                <a aria-label={sourceActionLabel(entry)} className={cn(RAIL_BUTTON, 'text-muted-foreground hover:text-foreground')} href={original} rel="noopener noreferrer" target="_blank">
                    <ExternalLinkIcon className="size-[1.125rem]" />
                </a>
            ) : null}
        </>
    );
}

function SaveButton({ save }: { save: EntrySave }) {
    const { saved, tags } = useSaveState(save);

    function toggle() {
        if (!saved) {
            save.save();
            announceSave(save);
            return;
        }
        announceRemove(save, tags);
    }

    return (
        <button
            aria-label={saved ? 'Remove from Library' : 'Save to Library'}
            aria-pressed={saved}
            className={cn(RAIL_BUTTON, saved ? 'text-primary' : 'text-muted-foreground hover:text-foreground')}
            onClick={toggle}
            type="button"
        >
            <BookmarkIcon className={cn('size-[1.125rem]', saved && '[&_path]:fill-current')} />
        </button>
    );
}

const RING_RADIUS = 8.25;
const RING_CIRCUMFERENCE = 2 * Math.PI * RING_RADIUS;

function ProgressRing({ progress }: { progress: number }) {
    return (
        <div aria-label="Reading progress" aria-valuemax={100} aria-valuemin={0} aria-valuenow={Math.round(progress * 100)} className="mt-2 size-[1.125rem]" role="progressbar">
            <svg aria-hidden="true" className="size-full -rotate-90" viewBox="0 0 18 18">
                <circle className="stroke-border" cx="9" cy="9" fill="none" r={RING_RADIUS} strokeWidth="1.5" />
                <circle
                    className="stroke-primary"
                    cx="9"
                    cy="9"
                    fill="none"
                    r={RING_RADIUS}
                    strokeDasharray={RING_CIRCUMFERENCE}
                    strokeDashoffset={RING_CIRCUMFERENCE * (1 - progress)}
                    strokeLinecap="round"
                    strokeWidth="1.5"
                />
            </svg>
        </div>
    );
}

function ProgressBar({ progress }: { progress: number }) {
    return (
        <div aria-label="Reading progress" aria-valuemax={100} aria-valuemin={0} aria-valuenow={Math.round(progress * 100)} className="h-px w-full overflow-hidden bg-border" role="progressbar">
            <div className="h-full w-full origin-left bg-primary" style={{ transform: `scaleX(${progress})` }} />
        </div>
    );
}

function useScrollProgress(): number {
    const [progress, setProgress] = useState(0);

    useEffect(() => {
        const compute = () => {
            const max = document.documentElement.scrollHeight - window.innerHeight;
            // Whole percents let React bail out on most scroll events instead of re-rendering every frame.
            setProgress(max <= 0 ? 1 : Math.round(Math.max(0, Math.min(1, window.scrollY / max)) * 100) / 100);
        };
        compute();
        window.addEventListener('scroll', compute, { passive: true });
        const observer = new ResizeObserver(compute);
        observer.observe(document.documentElement);
        return () => {
            window.removeEventListener('scroll', compute);
            observer.disconnect();
        };
    }, []);

    return progress;
}
