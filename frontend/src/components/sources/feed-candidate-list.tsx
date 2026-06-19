import { Tick02Icon } from '@hugeicons/core-free-icons';
import { HugeiconsIcon } from '@hugeicons/react';
import type { Ref } from 'react';
import { memo, useId } from 'react';

import { Collapsible, CollapsibleContent } from '@/components/ui/collapsible';
import { CreatableCombobox } from '@/components/ui/creatable-combobox';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import { cn } from '@/lib/utils';
import { youtubeShortsFreeFeedUrl } from '@/lib/youtube';

export type FeedCandidate = {
    feedUrl: string;
    title: string | null;
    siteUrl: string | null;
};

type Selection = {
    title: string;
    primary: boolean;
    tags: string[];
    excludeShorts: boolean;
};

// Stable empty-array reference so unselected cards keep referential equality
// across renders (the memo'd card compares props).
const EMPTY_TAGS: string[] = [];

type Props = {
    candidates: FeedCandidate[];
    existingByFeedUrl: Map<string, string | null>;
    selected: Record<string, Selection>;
    onToggle: (candidate: FeedCandidate) => void;
    onTitleChange: (feedUrl: string, title: string) => void;
    onPrimaryChange: (feedUrl: string, primary: boolean) => void;
    onTagsChange: (feedUrl: string, tags: string[]) => void;
    onExcludeShortsChange: (feedUrl: string, excludeShorts: boolean) => void;
    tagSuggestions: string[];
    firstCheckboxRef?: Ref<HTMLInputElement>;
    firstTitleInputRef?: Ref<HTMLInputElement>;
    'aria-labelledby'?: string;
};

export function FeedCandidateList({
    candidates,
    existingByFeedUrl,
    selected,
    onToggle,
    onTitleChange,
    onPrimaryChange,
    onTagsChange,
    onExcludeShortsChange,
    tagSuggestions,
    firstCheckboxRef,
    firstTitleInputRef,
    'aria-labelledby': ariaLabelledBy,
}: Props) {
    return (
        <div
            role="group"
            aria-labelledby={ariaLabelledBy}
            className="flex flex-col gap-2"
        >
            {candidates.map((candidate, index) => {
                const isExisting = existingByFeedUrl.has(candidate.feedUrl);
                const savedTitle = isExisting
                    ? (existingByFeedUrl.get(candidate.feedUrl) ?? null)
                    : null;
                const selection = selected[candidate.feedUrl];
                const isSelected = selection !== undefined;

                return (
                    <FeedCandidateCard
                        key={candidate.feedUrl}
                        candidate={candidate}
                        isExisting={isExisting}
                        savedTitle={savedTitle}
                        isSelected={isSelected}
                        selectedTitle={selection?.title ?? ''}
                        selectedPrimary={selection?.primary ?? false}
                        selectedTags={selection?.tags ?? EMPTY_TAGS}
                        selectedExcludeShorts={
                            selection?.excludeShorts ?? false
                        }
                        onToggle={onToggle}
                        onTitleChange={onTitleChange}
                        onPrimaryChange={onPrimaryChange}
                        onTagsChange={onTagsChange}
                        onExcludeShortsChange={onExcludeShortsChange}
                        tagSuggestions={tagSuggestions}
                        firstCheckboxRef={
                            index === 0 ? firstCheckboxRef : undefined
                        }
                        firstTitleInputRef={
                            index === 0 ? firstTitleInputRef : undefined
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
    isSelected: boolean;
    selectedTitle: string;
    selectedPrimary: boolean;
    selectedTags: string[];
    selectedExcludeShorts: boolean;
    onToggle: (candidate: FeedCandidate) => void;
    onTitleChange: (feedUrl: string, title: string) => void;
    onPrimaryChange: (feedUrl: string, primary: boolean) => void;
    onTagsChange: (feedUrl: string, tags: string[]) => void;
    onExcludeShortsChange: (feedUrl: string, excludeShorts: boolean) => void;
    tagSuggestions: string[];
    firstCheckboxRef?: Ref<HTMLInputElement>;
    firstTitleInputRef?: Ref<HTMLInputElement>;
};

const FeedCandidateCard = memo(function FeedCandidateCard({
    candidate,
    isExisting,
    savedTitle,
    isSelected,
    selectedTitle,
    selectedPrimary,
    selectedTags,
    selectedExcludeShorts,
    onToggle,
    onTitleChange,
    onPrimaryChange,
    onTagsChange,
    onExcludeShortsChange,
    tagSuggestions,
    firstCheckboxRef,
    firstTitleInputRef,
}: CardProps) {
    const inputId = useId();
    const titleId = useId();
    const tagsId = useId();
    const primaryId = useId();
    const excludeShortsId = useId();
    const shortsFreeUrl = youtubeShortsFreeFeedUrl(candidate.feedUrl);

    return (
        <div
            data-state={
                isExisting ? 'existing' : isSelected ? 'selected' : 'idle'
            }
            className={cn(
                'rounded-xl border border-border bg-muted',
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
                        ref={firstCheckboxRef}
                        id={inputId}
                        type="checkbox"
                        className={cn(
                            'peer size-4 cursor-pointer appearance-none rounded-sm border border-foreground/30 bg-background transition-colors',
                            'checked:border-primary checked:bg-primary',
                            'outline-none focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring focus-visible:outline-solid',
                            'disabled:cursor-not-allowed',
                        )}
                        checked={isSelected || isExisting}
                        disabled={isExisting}
                        onChange={() => onToggle(candidate)}
                    />
                    <HugeiconsIcon
                        icon={Tick02Icon}
                        className="pointer-events-none absolute inset-0 m-auto size-3 text-primary-foreground opacity-0 peer-checked:opacity-100"
                    />
                </span>
                <div className="flex min-w-0 flex-1 flex-col gap-1">
                    <span className="flex min-w-0 items-center gap-2">
                        <span className="truncate text-sm font-medium">
                            {(isExisting ? savedTitle : candidate.title) ??
                                candidate.feedUrl}
                        </span>
                        {isExisting && (
                            <span className="text-xs font-light text-muted-foreground">
                                Already added
                            </span>
                        )}
                    </span>
                    <span className="truncate text-xs text-muted-foreground">
                        {candidate.feedUrl}
                    </span>
                </div>
            </label>

            <Collapsible open={isSelected && !isExisting}>
                <CollapsibleContent className="overflow-hidden">
                    <div className="flex flex-col gap-4 border-t border-foreground/10 px-4 py-3">
                        <div className="flex flex-col gap-1.5">
                            <Label htmlFor={titleId} className="text-xs">
                                Title
                            </Label>
                            <Input
                                ref={firstTitleInputRef}
                                id={titleId}
                                type="text"
                                value={selectedTitle}
                                onChange={(event) =>
                                    onTitleChange(
                                        candidate.feedUrl,
                                        event.target.value,
                                    )
                                }
                            />
                        </div>

                        <div className="flex items-center justify-between gap-3">
                            <div className="flex flex-col gap-0.5">
                                <Label
                                    htmlFor={primaryId}
                                    className="cursor-pointer text-xs"
                                >
                                    Primary source
                                </Label>
                                <span className="text-xs font-light text-muted-foreground">
                                    Featured prominently in your digest.
                                </span>
                            </div>
                            <Switch
                                id={primaryId}
                                checked={selectedPrimary}
                                onCheckedChange={(checked) =>
                                    onPrimaryChange(candidate.feedUrl, checked)
                                }
                            />
                        </div>

                        {shortsFreeUrl && (
                            <div className="flex items-center justify-between gap-3">
                                <div className="flex flex-col gap-0.5">
                                    <Label
                                        htmlFor={excludeShortsId}
                                        className="cursor-pointer text-xs"
                                    >
                                        Exclude Shorts
                                    </Label>
                                    <span className="text-xs font-light text-muted-foreground">
                                        Subscribe to long-form uploads only.
                                    </span>
                                </div>
                                <Switch
                                    id={excludeShortsId}
                                    checked={selectedExcludeShorts}
                                    onCheckedChange={(checked) =>
                                        onExcludeShortsChange(
                                            candidate.feedUrl,
                                            checked,
                                        )
                                    }
                                />
                            </div>
                        )}

                        <div className="flex flex-col gap-1.5">
                            <Label htmlFor={tagsId} className="text-xs">
                                Tags
                            </Label>
                            <CreatableCombobox
                                id={tagsId}
                                value={selectedTags}
                                onValueChange={(tags) =>
                                    onTagsChange(candidate.feedUrl, tags)
                                }
                                suggestions={tagSuggestions}
                                placeholder="Add tags…"
                            />
                        </div>
                    </div>
                </CollapsibleContent>
            </Collapsible>
        </div>
    );
});
