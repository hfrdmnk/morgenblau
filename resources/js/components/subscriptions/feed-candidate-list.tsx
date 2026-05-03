import { Tick02Icon } from '@hugeicons/core-free-icons';
import { HugeiconsIcon } from '@hugeicons/react';
import type { Ref } from 'react';
import { useId } from 'react';

import { Collapsible, CollapsibleContent } from '@/components/ui/collapsible';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from '@/components/ui/select';
import { cn } from '@/lib/utils';

type FeedCandidate = App.Data.Feeds.ResolvedFeedData;
type SourceType = App.Enums.SourceType;

export type SelectedMeta = {
    title: string;
    source_type: SourceType;
};

export const SOURCE_TYPE_LABELS: Record<SourceType, string> = {
    rss: 'Website',
    video: 'Video',
    podcast: 'Podcast',
    microblog: 'Microblog',
};

type Props = {
    candidates: FeedCandidate[];
    existingByFeedUrl: Record<string, string | null>;
    selected: Record<string, SelectedMeta>;
    onToggle: (candidate: FeedCandidate) => void;
    onTitleChange: (feedUrl: string, title: string) => void;
    onSourceTypeChange: (feedUrl: string, type: SourceType) => void;
    containerRef?: Ref<HTMLDivElement>;
};

export function FeedCandidateList({
    candidates,
    existingByFeedUrl,
    selected,
    onToggle,
    onTitleChange,
    onSourceTypeChange,
    containerRef,
}: Props) {
    return (
        <div ref={containerRef} role="group" className="flex flex-col gap-2">
            {candidates.map((candidate) => {
                const isExisting = candidate.feed_url in existingByFeedUrl;
                const savedTitle = isExisting
                    ? existingByFeedUrl[candidate.feed_url]
                    : null;
                const meta = selected[candidate.feed_url];

                return (
                    <FeedCandidateCard
                        key={candidate.feed_url}
                        candidate={candidate}
                        isExisting={isExisting}
                        savedTitle={savedTitle}
                        meta={meta}
                        onToggle={() => onToggle(candidate)}
                        onTitleChange={(title) =>
                            onTitleChange(candidate.feed_url, title)
                        }
                        onSourceTypeChange={(type) =>
                            onSourceTypeChange(candidate.feed_url, type)
                        }
                    />
                );
            })}
        </div>
    );
}

type CardProps = {
    candidate: FeedCandidate;
    isExisting: boolean;
    savedTitle: string | null;
    meta: SelectedMeta | undefined;
    onToggle: () => void;
    onTitleChange: (title: string) => void;
    onSourceTypeChange: (type: SourceType) => void;
};

function FeedCandidateCard({
    candidate,
    isExisting,
    savedTitle,
    meta,
    onToggle,
    onTitleChange,
    onSourceTypeChange,
}: CardProps) {
    const inputId = useId();
    const titleId = useId();
    const typeId = useId();

    const isSelected = meta !== undefined;

    return (
        <div
            data-state={
                isExisting ? 'existing' : isSelected ? 'selected' : 'idle'
            }
            className={cn(
                'rounded-xl border border-border bg-gray-50 dark:bg-gray-900',
                isExisting && 'opacity-60',
            )}
        >
            <label
                htmlFor={inputId}
                className={cn(
                    'flex min-w-0 items-start gap-3 px-4 py-3',
                    isExisting ? 'cursor-not-allowed' : 'cursor-pointer',
                )}
            >
                <span className="relative mt-0.5 inline-flex shrink-0">
                    <input
                        id={inputId}
                        type="checkbox"
                        className={cn(
                            'peer size-4 cursor-pointer appearance-none rounded-sm border border-foreground/30 bg-background transition-colors',
                            'checked:border-foreground/60 checked:bg-foreground/70',
                            'focus-visible:border-ring focus-visible:outline-none',
                            'disabled:cursor-not-allowed',
                        )}
                        checked={isSelected || isExisting}
                        disabled={isExisting}
                        onChange={onToggle}
                    />
                    <HugeiconsIcon
                        icon={Tick02Icon}
                        className="pointer-events-none absolute inset-0 m-auto size-3 text-background opacity-0 peer-checked:opacity-100"
                    />
                </span>
                <div className="flex min-w-0 flex-1 flex-col gap-1">
                    <span className="flex min-w-0 items-center gap-2">
                        <span className="truncate text-sm font-medium">
                            {(isExisting ? savedTitle : candidate.title) ??
                                candidate.feed_url}
                        </span>
                        {isExisting && (
                            <span className="font-handwritten text-xs text-muted-foreground">
                                Already added
                            </span>
                        )}
                    </span>
                    <span className="truncate text-xs text-muted-foreground">
                        {candidate.feed_url}
                    </span>
                </div>
            </label>

            <Collapsible open={isSelected && !isExisting}>
                <CollapsibleContent className="overflow-hidden">
                    <div className="flex flex-col gap-3 border-t border-foreground/10 px-4 py-3">
                        <div className="flex flex-col gap-1.5">
                            <Label htmlFor={titleId} className="text-xs">
                                Title
                            </Label>
                            <Input
                                id={titleId}
                                type="text"
                                value={meta?.title ?? ''}
                                onChange={(event) =>
                                    onTitleChange(event.target.value)
                                }
                            />
                        </div>
                        <div className="flex flex-col gap-1.5">
                            <Label htmlFor={typeId} className="text-xs">
                                Source type
                            </Label>
                            <Select
                                value={meta?.source_type ?? 'rss'}
                                onValueChange={(value) =>
                                    onSourceTypeChange(value as SourceType)
                                }
                            >
                                <SelectTrigger id={typeId}>
                                    <SelectValue>
                                        {(value) =>
                                            SOURCE_TYPE_LABELS[
                                                value as SourceType
                                            ] ?? SOURCE_TYPE_LABELS.rss
                                        }
                                    </SelectValue>
                                </SelectTrigger>
                                <SelectContent>
                                    {(
                                        Object.keys(
                                            SOURCE_TYPE_LABELS,
                                        ) as SourceType[]
                                    ).map((type) => (
                                        <SelectItem key={type} value={type}>
                                            {SOURCE_TYPE_LABELS[type]}
                                        </SelectItem>
                                    ))}
                                </SelectContent>
                            </Select>
                        </div>
                    </div>
                </CollapsibleContent>
            </Collapsible>
        </div>
    );
}
