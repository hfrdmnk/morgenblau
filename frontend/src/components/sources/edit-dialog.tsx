import { SpinnerIcon } from '@proicons/react';
import { useCallback, useState } from 'react';
import type { FormEvent } from 'react';

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
    initialFeedUrl: string;
    tagSuggestions: string[];
    onSave: (patch: SourcePatch) => Promise<boolean>;
};

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
    // Exclude-Shorts applies only to YouTube feeds; its state is encoded in the feed URL, not a stored flag.
    const channelFeedUrl = youtubeChannelFeedUrl(initialFeedUrl);
    const isYoutube = channelFeedUrl !== null;
    const initialExcludeShorts = isYoutubeShortsFreeFeedUrl(initialFeedUrl);

    const [title, setTitle] = useState(initialTitle);
    const [primary, setPrimary] = useState(initialPrimary);
    const [tags, setTags] = useState<string[]>(initialTags);
    const [excludeShorts, setExcludeShorts] = useState(initialExcludeShorts);
    const [saving, setSaving] = useState(false);

    const handleOpenChangeComplete = useCallback(
        (nextOpen: boolean) => {
            if (!nextOpen) {
                setTitle(initialTitle);
                setPrimary(initialPrimary);
                setTags(initialTags);
                setExcludeShorts(initialExcludeShorts);
            }
        },
        [initialTitle, initialPrimary, initialTags, initialExcludeShorts],
    );

    const submit = async (event: FormEvent) => {
        event.preventDefault();
        if (saving) return;
        // Keep the existing title rather than wiping it to empty; the backend no-ops the PATCH if unchanged.
        const nextTitle = title.trim() || initialTitle;
        // Re-point the feed only when the Shorts toggle moved, so an untouched save never triggers a re-fetch.
        let feedUrl: string | undefined;
        if (
            isYoutube &&
            channelFeedUrl &&
            excludeShorts !== initialExcludeShorts
        ) {
            feedUrl = excludeShorts
                ? (youtubeShortsFreeFeedUrl(channelFeedUrl) ?? channelFeedUrl)
                : channelFeedUrl;
        }
        setSaving(true);
        const ok = await onSave({ title: nextTitle, primary, tags, feedUrl });
        setSaving(false);
        if (ok) onOpenChange(false);
    };

    return (
        <Dialog
            open={open}
            onOpenChange={onOpenChange}
            onOpenChangeComplete={handleOpenChangeComplete}
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
                    <div className="space-y-2">
                        <Label htmlFor="source-title" className="text-xs">
                            Title
                        </Label>
                        <Input
                            id="source-title"
                            autoFocus
                            value={title}
                            onChange={(e) => setTitle(e.target.value)}
                            placeholder="Source title"
                        />
                    </div>

                    <div className="flex items-center justify-between gap-3">
                        <div className="flex flex-col gap-0.5">
                            <Label
                                htmlFor="source-primary"
                                className="cursor-pointer text-xs"
                            >
                                Primary source
                            </Label>
                            <span className="text-xs font-light text-muted-foreground">
                                Featured prominently in your digest.
                            </span>
                        </div>
                        <Switch
                            id="source-primary"
                            checked={primary}
                            onCheckedChange={(checked) => setPrimary(checked)}
                        />
                    </div>

                    {isYoutube && (
                        <div className="flex items-center justify-between gap-3">
                            <div className="flex flex-col gap-0.5">
                                <Label
                                    htmlFor="source-exclude-shorts"
                                    className="cursor-pointer text-xs"
                                >
                                    Exclude Shorts
                                </Label>
                                <span className="text-xs font-light text-muted-foreground">
                                    Subscribe to long-form uploads only.
                                </span>
                            </div>
                            <Switch
                                id="source-exclude-shorts"
                                checked={excludeShorts}
                                onCheckedChange={(checked) =>
                                    setExcludeShorts(checked)
                                }
                            />
                        </div>
                    )}

                    <div className="space-y-2">
                        <Label htmlFor="source-tags" className="text-xs">
                            Tags
                        </Label>
                        <CreatableCombobox
                            id="source-tags"
                            value={tags}
                            onValueChange={setTags}
                            suggestions={tagSuggestions}
                            placeholder="Add tags…"
                        />
                    </div>

                    <DialogFooter>
                        <Button
                            type="button"
                            variant="secondary"
                            onClick={() => onOpenChange(false)}
                            disabled={saving}
                        >
                            Cancel
                        </Button>
                        <Button type="submit" disabled={saving}>
                            {saving ? (
                                <>
                                    <SpinnerIcon className="motion-safe:animate-spin" />
                                    Saving…
                                </>
                            ) : (
                                'Save'
                            )}
                        </Button>
                    </DialogFooter>
                </form>
            </DialogContent>
        </Dialog>
    );
}
