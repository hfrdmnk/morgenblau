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

export type FeedCandidate = {
    feed_url: string;
    title: string | null;
    site_url: string | null;
    source_type: SourceType;
};

export type SourceType = 'rss' | 'video' | 'podcast' | 'microblog';

export const SOURCE_TYPE_LABELS: Record<SourceType, string> = {
    rss: 'Website',
    video: 'Video',
    podcast: 'Podcast',
    microblog: 'Microblog',
};

type Props = {
    candidates: FeedCandidate[];
    selectedFeedUrl: string | null;
    onSelect: (feedUrl: string) => void;
    title: string;
    onTitleChange: (title: string) => void;
    sourceType: SourceType;
    onSourceTypeChange: (type: SourceType) => void;
};

export function FeedCandidateList({
    candidates,
    selectedFeedUrl,
    onSelect,
    title,
    onTitleChange,
    sourceType,
    onSourceTypeChange,
}: Props) {
    const groupName = useId();

    return (
        <div role="radiogroup" className="flex flex-col gap-2">
            {candidates.map((candidate) => {
                const isSelected = candidate.feed_url === selectedFeedUrl;

                return (
                    <FeedCandidateCard
                        key={candidate.feed_url}
                        candidate={candidate}
                        groupName={groupName}
                        isSelected={isSelected}
                        onSelect={() => onSelect(candidate.feed_url)}
                        title={title}
                        onTitleChange={onTitleChange}
                        sourceType={sourceType}
                        onSourceTypeChange={onSourceTypeChange}
                    />
                );
            })}
        </div>
    );
}

type CardProps = {
    candidate: FeedCandidate;
    groupName: string;
    isSelected: boolean;
    onSelect: () => void;
    title: string;
    onTitleChange: (title: string) => void;
    sourceType: SourceType;
    onSourceTypeChange: (type: SourceType) => void;
};

function FeedCandidateCard({
    candidate,
    groupName,
    isSelected,
    onSelect,
    title,
    onTitleChange,
    sourceType,
    onSourceTypeChange,
}: CardProps) {
    const inputId = useId();
    const titleId = useId();
    const typeId = useId();

    return (
        <div
            data-state={isSelected ? 'selected' : 'idle'}
            className="rounded-xl border border-border bg-gray-50 dark:bg-gray-900"
        >
            <label
                htmlFor={inputId}
                className="flex min-w-0 cursor-pointer items-start gap-3 px-4 py-3"
            >
                <input
                    id={inputId}
                    type="radio"
                    name={groupName}
                    className={cn(
                        'mt-0.5 size-4 shrink-0 cursor-pointer appearance-none rounded-full border border-foreground/30 bg-background transition-colors',
                        'checked:border-foreground/60 checked:bg-foreground/70',
                        'checked:shadow-[inset_0_0_0_2.5px_var(--background)]',
                        'focus-visible:ring-2 focus-visible:ring-foreground/20 focus-visible:ring-offset-2 focus-visible:ring-offset-gray-50 focus-visible:outline-none dark:focus-visible:ring-offset-gray-900',
                    )}
                    checked={isSelected}
                    onChange={onSelect}
                />
                <div className="flex min-w-0 flex-1 flex-col gap-1">
                    <span className="truncate text-sm font-medium">
                        {candidate.title ?? candidate.feed_url}
                    </span>
                    <span className="truncate text-xs text-muted-foreground">
                        {candidate.feed_url}
                    </span>
                </div>
            </label>

            <Collapsible open={isSelected}>
                <CollapsibleContent className="overflow-hidden data-open:animate-in data-open:fade-in-0 data-closed:animate-out data-closed:fade-out-0">
                    <div className="flex flex-col gap-3 border-t border-foreground/10 px-4 py-3">
                        <div className="flex flex-col gap-1.5">
                            <Label htmlFor={titleId} className="text-xs">
                                Title
                            </Label>
                            <Input
                                id={titleId}
                                type="text"
                                value={title}
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
                                value={sourceType}
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
