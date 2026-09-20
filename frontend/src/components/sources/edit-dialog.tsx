import { SpinnerIcon } from '@proicons/react';
import { useCallback, useState, type FormEvent } from 'react';

import { Button } from '@/components/ui/button';
import { CreatableCombobox } from '@/components/ui/creatable-combobox';
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogHeader,
    DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import {
    isYoutubeShortsFreeFeedUrl,
    youtubeChannelFeedUrl,
    youtubeShortsFreeFeedUrl,
} from '@/lib/youtube';

export type SourcePatch = {
    title: string;
    primary: boolean;
    tags: string[];
    // Set only when the feed URL changes (the YouTube exclude-Shorts toggle).
    feedUrl?: string;
};

type Props = {
    open: boolean;
    onOpenChange: (open: boolean) => void;
    initialTitle: string;
    initialPrimary: boolean;
    initialTags: string[];
    initialFeedUrl?: string;
    tagSuggestions: string[];
    onSave: (patch: SourcePatch) => Promise<boolean>;
};

function sourceFeedSettings(initialFeedUrl: string | undefined) {
    const channelFeedUrl = initialFeedUrl
        ? youtubeChannelFeedUrl(initialFeedUrl)
        : null;
    return {
        channelFeedUrl,
        initialExcludeShorts: initialFeedUrl
            ? isYoutubeShortsFreeFeedUrl(initialFeedUrl)
            : false,
    };
}

function shortsFreeFeedUrl(channelFeedUrl: string) {
    const feedUrl = youtubeShortsFreeFeedUrl(channelFeedUrl);
    return feedUrl === null ? channelFeedUrl : feedUrl;
}

function changedFeedUrl(
    channelFeedUrl: string | null,
    excludeShorts: boolean,
    initialExcludeShorts: boolean,
) {
    if (!channelFeedUrl) return undefined;
    if (excludeShorts === initialExcludeShorts) return undefined;
    if (!excludeShorts) return channelFeedUrl;
    return shortsFreeFeedUrl(channelFeedUrl);
}

function TitleField({
    title,
    onChange,
}: {
    title: string;
    onChange: (title: string) => void;
}) {
    return (
        <div className="space-y-2">
            <Label htmlFor="source-title" className="text-xs">
                Title
            </Label>
            <Input
                id="source-title"
                autoFocus
                value={title}
                onChange={(event) => onChange(event.target.value)}
                placeholder="Source title"
            />
        </div>
    );
}

function SwitchField({
    id,
    label,
    description,
    checked,
    onCheckedChange,
}: {
    id: string;
    label: string;
    description: string;
    checked: boolean;
    onCheckedChange: (checked: boolean) => void;
}) {
    return (
        <div className="flex items-center justify-between gap-3">
            <div className="flex flex-col gap-0.5">
                <Label htmlFor={id} className="cursor-pointer text-xs">
                    {label}
                </Label>
                <span className="text-xs font-light text-muted-foreground">
                    {description}
                </span>
            </div>
            <Switch
                id={id}
                checked={checked}
                onCheckedChange={onCheckedChange}
            />
        </div>
    );
}

function TagsField({
    tags,
    suggestions,
    onChange,
}: {
    tags: string[];
    suggestions: string[];
    onChange: (tags: string[]) => void;
}) {
    return (
        <div className="space-y-2">
            <Label htmlFor="source-tags" className="text-xs">
                Tags
            </Label>
            <CreatableCombobox
                id="source-tags"
                value={tags}
                onValueChange={onChange}
                suggestions={suggestions}
                placeholder="Add tags…"
            />
        </div>
    );
}

function SaveButton({ saving }: { saving: boolean }) {
    if (!saving) return <Button type="submit">Save</Button>;
    return (
        <Button type="submit" disabled>
            <SpinnerIcon className="motion-safe:animate-spin" />
            Saving…
        </Button>
    );
}

// EditSourceDialog mirrors AddSourceDialog's chrome; delete lives on the row, not here.
export function EditSourceDialog({
    open,
    onOpenChange,
    initialTitle,
    initialPrimary,
    initialTags,
    initialFeedUrl,
    tagSuggestions,
    onSave,
}: Props) {
    const { channelFeedUrl, initialExcludeShorts } =
        sourceFeedSettings(initialFeedUrl);
    const [title, setTitle] = useState(initialTitle);
    const [primary, setPrimary] = useState(initialPrimary);
    const [tags, setTags] = useState<string[]>(initialTags);
    const [excludeShorts, setExcludeShorts] = useState(initialExcludeShorts);
    const [saving, setSaving] = useState(false);

    const reset = useCallback(
        (nextOpen: boolean) => {
            if (nextOpen) return;
            setTitle(initialTitle);
            setPrimary(initialPrimary);
            setTags(initialTags);
            setExcludeShorts(initialExcludeShorts);
        },
        [initialTitle, initialPrimary, initialTags, initialExcludeShorts],
    );

    const submit = async (event: FormEvent) => {
        event.preventDefault();
        if (saving) return;
        setSaving(true);
        const ok = await onSave({
            title: title.trim() || initialTitle,
            primary,
            tags,
            feedUrl: changedFeedUrl(
                channelFeedUrl,
                excludeShorts,
                initialExcludeShorts,
            ),
        });
        setSaving(false);
        if (ok) onOpenChange(false);
    };

    return (
        <Dialog
            open={open}
            onOpenChange={onOpenChange}
            onOpenChangeComplete={reset}
        >
            <DialogContent>
                <DialogHeader>
                    <DialogTitle>Edit source</DialogTitle>
                    <DialogDescription>
                        Rename it, mark it primary, or organise it with tags.
                    </DialogDescription>
                </DialogHeader>
                <form
                    onSubmit={submit}
                    noValidate
                    className="flex flex-col gap-5"
                >
                    <TitleField title={title} onChange={setTitle} />
                    <SwitchField
                        id="source-primary"
                        label="Primary source"
                        description="Featured prominently in your digest."
                        checked={primary}
                        onCheckedChange={setPrimary}
                    />
                    {channelFeedUrl ? (
                        <SwitchField
                            id="source-exclude-shorts"
                            label="Exclude Shorts"
                            description="Subscribe to long-form uploads only."
                            checked={excludeShorts}
                            onCheckedChange={setExcludeShorts}
                        />
                    ) : null}
                    <TagsField
                        tags={tags}
                        suggestions={tagSuggestions}
                        onChange={setTags}
                    />
                    <DialogFooter>
                        <Button
                            type="button"
                            variant="secondary"
                            onClick={() => onOpenChange(false)}
                            disabled={saving}
                        >
                            Cancel
                        </Button>
                        <SaveButton saving={saving} />
                    </DialogFooter>
                </form>
            </DialogContent>
        </Dialog>
    );
}
