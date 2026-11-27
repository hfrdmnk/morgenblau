import { useEffect, useState, type ReactElement } from 'react';

import { IconCrossfade } from '@/components/icon-crossfade';
import { BookmarkIcon, CheckIcon, CopyIcon, ExternalLinkIcon } from '@/components/icons';
import { RailBar, RailButton, RailColumn, RailLink } from '@/components/ui/reading-rail';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { Text } from '@/design/text';
import { announceRemove, announceSave, dismissSaveToasts } from '@/lib/save-announce';
import { useCopy } from '@/hooks/use-copy';
import { useSaveState } from '@/hooks/use-entry-save';
import type { EntrySave } from '@/lib/entry-save';
import { httpURL, sourceActionLabel, type ReaderEntry } from '@/lib/reader';

export function ReaderRail({ entry, save }: { entry: ReaderEntry; save: EntrySave | null }) {
    const progress = useScrollProgress();
    useEffect(() => dismissSaveToasts, []);

    return (
        <TooltipProvider>
            <RailColumn label="Reader actions" progress={progress}>
                <RailActions entry={entry} save={save} side="left" />
            </RailColumn>
            <RailBar label="Reader actions" progress={progress}>
                <RailActions entry={entry} save={save} side="top" />
            </RailBar>
        </TooltipProvider>
    );
}

type TooltipSide = 'left' | 'top';

function RailActions({ entry, save, side }: { entry: ReaderEntry; save: EntrySave | null; side: TooltipSide }) {
    const original = httpURL(entry.url);
    const label = sourceActionLabel(entry);
    return (
        <>
            {save ? <SaveButton save={save} side={side} /> : null}
            {original ? (
                <RailTooltip label={label} side={side}>
                    <RailLink aria-label={label} href={original} rel="noopener noreferrer" target="_blank">
                        <ExternalLinkIcon size="lg" />
                    </RailLink>
                </RailTooltip>
            ) : null}
            {original ? <CopyLinkButton side={side} url={original} /> : null}
        </>
    );
}

function RailTooltip({ label, side, children }: { label: string; side: TooltipSide; children: ReactElement }) {
    return (
        <Tooltip>
            <TooltipTrigger render={children} />
            <TooltipContent side={side}>{label}</TooltipContent>
        </Tooltip>
    );
}

function SaveButton({ save, side }: { save: EntrySave; side: TooltipSide }) {
    const { saved, tags } = useSaveState(save);

    function toggle() {
        if (!saved) {
            save.save();
            announceSave(save);
            return;
        }
        announceRemove(save, tags);
    }
    const label = saved ? 'Remove from Library' : 'Save to Library';

    return (
        <RailTooltip label={label} side={side}>
            <RailButton aria-label={label} aria-pressed={saved} onClick={toggle} tone={saved ? 'on' : 'idle'}>
                <BookmarkIcon size="lg" />
            </RailButton>
        </RailTooltip>
    );
}

const COPY_LINK_FAILURE = {
    title: 'Couldn’t copy the link',
    description: 'Open the original and copy its address instead.',
};

const COPY_LINK = {
    idle: { label: 'Copy link', icon: CopyIcon, tone: 'idle', announcement: '' },
    copied: { label: 'Link copied', icon: CheckIcon, tone: 'success', announcement: 'Link copied.' },
} as const;

function CopyLinkButton({ url, side }: { url: string; side: TooltipSide }) {
    const { copied, copy, instant } = useCopy(url, COPY_LINK_FAILURE);
    const state = copied ? COPY_LINK.copied : COPY_LINK.idle;

    return (
        <RailTooltip label={state.label} side={side}>
            <RailButton aria-label={state.label} onClick={copy} tone={state.tone}>
                <IconCrossfade size="lg" icon={state.icon} instant={instant} name={state.label} />
                <Text aria-live="polite" as="span" visuallyHidden>
                    {state.announcement}
                </Text>
            </RailButton>
        </RailTooltip>
    );
}

function useScrollProgress(): number {
    const [progress, setProgress] = useState(0);

    useEffect(() => {
        const compute = () => {
            const max = document.documentElement.scrollHeight - window.innerHeight;
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
