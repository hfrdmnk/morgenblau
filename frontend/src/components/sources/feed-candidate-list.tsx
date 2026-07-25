import { CheckmarkIcon } from '@proicons/react';
import type { Ref } from 'react';
import { memo, useId } from 'react';

import { Badge } from '@/components/ui/badge';
import { Collapsible, CollapsibleContent } from '@/components/ui/collapsible';
import { CreatableCombobox } from '@/components/ui/creatable-combobox';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import {
    Tooltip,
    TooltipContent,
    TooltipProvider,
    TooltipTrigger,
} from '@/components/ui/tooltip';
import { candidateKey, type FeedCandidate } from '@/lib/candidates';
import { cn } from '@/lib/utils';
import { youtubeShortsFreeFeedUrl } from '@/lib/youtube';

type Selection = {
    title: string;
    primary: boolean;
    tags: string[];
    excludeShorts: boolean;
};

// Stable empty-array reference so unselected cards keep referential equality across renders (memo compares props).
const EMPTY_TAGS: string[] = [];

type Props = {
    candidates: FeedCandidate[];
    existingByFeedUrl: Map<string, string | null>;
    // Candidate keys blocked because their cross-kind sibling is selected.
    siblingBlocked: Set<string>;
    selected: Record<string, Selection>;
    onToggle: (candidate: FeedCandidate) => void;
    onTitleChange: (key: string, title: string) => void;
    onPrimaryChange: (key: string, primary: boolean) => void;
    onTagsChange: (key: string, tags: string[]) => void;
    onExcludeShortsChange: (key: string, excludeShorts: boolean) => void;
    tagSuggestions: string[];
    firstCheckboxRef?: Ref<HTMLInputElement>;
    firstTitleInputRef?: Ref<HTMLInputElement>;
    'aria-labelledby'?: string;
};

export function FeedCandidateList({
    candidates,
    existingByFeedUrl,
    siblingBlocked,
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
        <TooltipProvider>
            <div
                role="group"
                aria-labelledby={ariaLabelledBy}
                className="flex flex-col gap-2"
            >
                {candidates.map((candidate, index) => {
                    const key = candidateKey(candidate);
                    const isExisting = existingByFeedUrl.has(key);
                    const savedTitle = isExisting
                        ? (existingByFeedUrl.get(key) ?? null)
                        : null;
                    const selection = selected[key];
                    const isSelected = selection !== undefined;

                    return (
                        <FeedCandidateCard
                            key={key}
                            candidate={candidate}
                            isExisting={isExisting}
                            savedTitle={savedTitle}
                            isSiblingBlocked={siblingBlocked.has(key)}
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
        </TooltipProvider>
    );
}

type CardProps = {
    candidate: FeedCandidate;
    isExisting: boolean;
    savedTitle: string | null;
    isSiblingBlocked: boolean;
    isSelected: boolean;
    selectedTitle: string;
    selectedPrimary: boolean;
    selectedTags: string[];
    selectedExcludeShorts: boolean;
    onToggle: (candidate: FeedCandidate) => void;
    onTitleChange: (key: string, title: string) => void;
    onPrimaryChange: (key: string, primary: boolean) => void;
    onTagsChange: (key: string, tags: string[]) => void;
    onExcludeShortsChange: (key: string, excludeShorts: boolean) => void;
    tagSuggestions: string[];
    firstCheckboxRef?: Ref<HTMLInputElement>;
    firstTitleInputRef?: Ref<HTMLInputElement>;
};

function subscribedViaLabel(via: { kind: string; title?: string }): string {
    if (via.title) {
        return `Already subscribed via ${via.title}`;
    }
    return via.kind === 'standardfeed'
        ? 'Already subscribed via ATProto'
        : 'Already subscribed via RSS';
}

const FeedCandidateCard = memo(function FeedCandidateCard({
    candidate,
    isExisting,
    savedTitle,
    isSiblingBlocked,
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
    const key = candidateKey(candidate);
    const shortsFreeUrl = candidate.feedUrl
        ? youtubeShortsFreeFeedUrl(candidate.feedUrl)
        : null;
    const isStandard = candidate.kind === 'standardfeed';
    const isDisabled =
        isExisting || candidate.subscribedVia !== undefined || isSiblingBlocked;

    return (
        <div
            data-state={
                isDisabled ? 'existing' : isSelected ? 'selected' : 'idle'
            }
            className="rounded-xl bg-muted"
        >
            <label
                htmlFor={inputId}
                className={cn(
                    'flex min-w-0 items-start gap-3 px-4 py-3',
                    isDisabled ? 'cursor-not-allowed' : 'cursor-pointer',
                )}
            >
                <span
                    className={cn(
                        'relative mt-0.5 inline-flex shrink-0',
                        isDisabled && 'opacity-60',
                    )}
                >
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
                        disabled={isDisabled}
                        onChange={() => onToggle(candidate)}
                    />
                    <CheckmarkIcon
                        className="pointer-events-none absolute inset-0 m-auto size-3 text-primary-foreground opacity-0 peer-checked:opacity-100"
                    />
                </span>
                <div className="flex min-w-0 flex-1 flex-col gap-1">
                    <span className="flex min-w-0 items-center gap-2">
                        <span
                            className={cn(
                                'truncate text-sm font-medium',
                                isDisabled && 'opacity-60',
                            )}
                        >
                            {(isExisting ? savedTitle : candidate.title) ??
                                candidate.feedUrl ??
                                candidate.siteUrl ??
                                key}
                        </span>
                        {isExisting && <Badge>Already added</Badge>}
                        {!isExisting && candidate.subscribedVia && (
                            <Badge>
                                {subscribedViaLabel(candidate.subscribedVia)}
                            </Badge>
                        )}
                        {!isExisting && !candidate.subscribedVia && isSiblingBlocked && (
                            <Badge>Same site selected</Badge>
                        )}
                        {isStandard &&
                            !isExisting &&
                            !candidate.subscribedVia && (
                                <Tooltip>
                                    <TooltipTrigger
                                        render={
                                            <Badge
                                                tabIndex={0}
                                                className="cursor-help"
                                            >
                                                <span aria-hidden className="text-xs leading-none font-medium">
                                                    @
                                                </span>
                                                Subscribe via ATProto
                                            </Badge>
                                        }
                                    />
                                    <TooltipContent>
                                        This subscription lives in your own
                                        account: it travels with you across
                                        apps, and anything you share reaches
                                        the whole Atmosphere.
                                    </TooltipContent>
                                </Tooltip>
                            )}
                    </span>
                    <span
                        className={cn(
                            'truncate text-xs text-muted-foreground',
                            isDisabled && 'opacity-60',
                        )}
                    >
                        {candidate.feedUrl ?? candidate.siteUrl ?? key}
                    </span>
                </div>
            </label>

            <Collapsible open={isSelected && !isDisabled}>
                <CollapsibleContent className="overflow-hidden">
                    <div className="flex flex-col gap-4 border-t border-border px-4 py-3">
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
                                    onTitleChange(key, event.target.value)
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
                                    onPrimaryChange(key, checked)
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
                                        onExcludeShortsChange(key, checked)
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
                                    onTagsChange(key, tags)
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
