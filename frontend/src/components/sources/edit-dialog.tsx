import { Loading03Icon } from '@hugeicons/core-free-icons';
import { HugeiconsIcon } from '@hugeicons/react';
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

export type SourcePatch = {
    title: string;
    primary: boolean;
    tags: string[];
};

type Props = {
    open: boolean;
    onOpenChange: (open: boolean) => void;
    initialTitle: string;
    initialPrimary: boolean;
    initialTags: string[];
    tagSuggestions: string[];
    onSave: (patch: SourcePatch) => Promise<boolean>;
};

// EditSourceDialog mirrors AddSourceDialog's chrome — title, a primary toggle,
// and tags. Delete lives on the row, not here.
export function EditSourceDialog({
    open,
    onOpenChange,
    initialTitle,
    initialPrimary,
    initialTags,
    tagSuggestions,
    onSave,
}: Props) {
    const [title, setTitle] = useState(initialTitle);
    const [primary, setPrimary] = useState(initialPrimary);
    const [tags, setTags] = useState<string[]>(initialTags);
    const [saving, setSaving] = useState(false);

    const handleOpenChangeComplete = useCallback(
        (nextOpen: boolean) => {
            if (!nextOpen) {
                setTitle(initialTitle);
                setPrimary(initialPrimary);
                setTags(initialTags);
            }
        },
        [initialTitle, initialPrimary, initialTags],
    );

    const submit = async (event: FormEvent) => {
        event.preventDefault();
        if (saving) return;
        // Keep the existing title rather than wiping it to empty; the backend
        // no-ops the PATCH if title, primary, and tags are all unchanged.
        const nextTitle = title.trim() || initialTitle;
        setSaving(true);
        const ok = await onSave({ title: nextTitle, primary, tags });
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

                    <label className="flex cursor-pointer items-center justify-between gap-3">
                        <span className="flex flex-col gap-0.5">
                            <span className="text-sm">Primary source</span>
                            <span className="text-xs font-light text-muted-foreground">
                                Featured prominently in your digest.
                            </span>
                        </span>
                        <Switch
                            checked={primary}
                            onCheckedChange={(checked) => setPrimary(checked)}
                        />
                    </label>

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
                                    <HugeiconsIcon
                                        icon={Loading03Icon}
                                        className="motion-safe:animate-spin"
                                    />
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
