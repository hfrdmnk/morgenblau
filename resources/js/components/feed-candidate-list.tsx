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
    rss: 'RSS',
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
            className={cn(
                'rounded-xl ring-1 ring-foreground/10 transition-colors',
                isSelected && 'ring-ring',
            )}
        >
            <label
                htmlFor={inputId}
                className="flex cursor-pointer items-start gap-3 px-4 py-3"
            >
                <input
                    id={inputId}
                    type="radio"
                    name={groupName}
                    className="mt-1.5 size-4 cursor-pointer accent-ring"
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
                <span className="rounded-full bg-muted/40 px-2 py-0.5 text-xs text-muted-foreground">
                    {SOURCE_TYPE_LABELS[candidate.source_type]}
                </span>
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
                                <SelectTrigger id={typeId} className="w-full">
                                    <SelectValue />
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
