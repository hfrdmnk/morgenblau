import { Loading03Icon } from '@hugeicons/core-free-icons';
import { HugeiconsIcon } from '@hugeicons/react';
import { useCallback, useState } from 'react';
import type { FormEvent } from 'react';

import { Button } from '@/components/ui/button';
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

type Props = {
    open: boolean;
    onOpenChange: (open: boolean) => void;
    initialTitle: string;
    onSave: (title: string) => Promise<boolean>;
};

// EditSourceDialog mirrors AddSourceDialog's chrome — single title field with
// Cancel / Save in the footer. Delete lives on the row, not here.
export function EditSourceDialog({
    open,
    onOpenChange,
    initialTitle,
    onSave,
}: Props) {
    const [draft, setDraft] = useState(initialTitle);
    const [saving, setSaving] = useState(false);

    const handleOpenChangeComplete = useCallback(
        (nextOpen: boolean) => {
            if (!nextOpen) setDraft(initialTitle);
        },
        [initialTitle],
    );

    const submit = async (event: FormEvent) => {
        event.preventDefault();
        if (saving) return;
        const next = draft.trim();
        if (!next || next === initialTitle) {
            onOpenChange(false);
            return;
        }
        setSaving(true);
        const ok = await onSave(next);
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
                        Rename it however reads best to you.
                    </DialogDescription>
                </DialogHeader>

                <form onSubmit={submit} className="flex flex-col gap-5">
                    <div className="space-y-2">
                        <Label htmlFor="source-title" className="sr-only">
                            Title
                        </Label>
                        <Input
                            id="source-title"
                            autoFocus
                            value={draft}
                            onChange={(e) => setDraft(e.target.value)}
                            placeholder="Source title"
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
