import { SpinnerIcon } from '@proicons/react';
import type { FormEvent } from 'react';

import { InputError } from '@/components/input-error';
import { ReauthNotice } from '@/components/reauth-notice';
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
import { useSubscribeDialog } from '@/hooks/use-subscribe-dialog';
import { type DiscoverSourceCard } from '@/lib/discover';

type Props = {
    source: DiscoverSourceCard | null;
    open: boolean;
    onOpenChange: (open: boolean) => void;
    onSubscribed: () => void;
};

// Single-candidate subscribe dialog for Discover cards: no URL-discovery phase, unlike AddSourceDialog.
export function SubscribeDialog({
    source,
    open,
    onOpenChange,
    onSubscribed,
}: Props) {
    const form = useSubscribeDialog(source, open, onSubscribed);

    const submit = (event: FormEvent) => {
        event.preventDefault();
        form.submit();
    };

    return (
        <Dialog
            open={open}
            onOpenChange={onOpenChange}
            onOpenChangeComplete={form.onOpenChangeComplete}
        >
            <DialogContent>
                <DialogHeader>
                    <DialogTitle>Add source</DialogTitle>
                    <DialogDescription>
                        Rename it, mark it primary, or add tags before it
                        joins your sources.
                    </DialogDescription>
                </DialogHeader>

                <form
                    onSubmit={submit}
                    noValidate
                    className="flex flex-col gap-5"
                >
                    <div className="space-y-2">
                        <Label htmlFor="subscribe-title" className="text-xs">
                            Title
                        </Label>
                        <Input
                            id="subscribe-title"
                            autoFocus
                            value={form.title}
                            onChange={(e) => form.setTitle(e.target.value)}
                            aria-invalid={form.titleInvalid}
                            placeholder="Source title"
                        />
                        <InputError message={form.titleError} />
                    </div>

                    <div className="flex items-center justify-between gap-3">
                        <div className="flex flex-col gap-0.5">
                            <Label
                                htmlFor="subscribe-primary"
                                className="cursor-pointer text-xs"
                            >
                                Primary source
                            </Label>
                            <span className="text-xs font-light text-muted-foreground">
                                Featured prominently in your digest.
                            </span>
                        </div>
                        <Switch
                            id="subscribe-primary"
                            checked={form.primary}
                            onCheckedChange={form.setPrimary}
                        />
                    </div>

                    <div className="space-y-2">
                        <Label htmlFor="subscribe-tags" className="text-xs">
                            Tags
                        </Label>
                        <CreatableCombobox
                            id="subscribe-tags"
                            value={form.tags}
                            onValueChange={form.setTags}
                            suggestions={form.tagSuggestions}
                            placeholder="Add tags…"
                        />
                    </div>

                    {form.needsReauth && <ReauthNotice />}

                    {form.topLevelError && (
                        <InputError message={form.topLevelError} />
                    )}

                    <DialogFooter>
                        <Button
                            type="button"
                            variant="secondary"
                            onClick={() => onOpenChange(false)}
                            disabled={form.submitting}
                        >
                            Cancel
                        </Button>
                        <Button type="submit" disabled={form.submitting}>
                            {form.submitting ? (
                                <>
                                    <SpinnerIcon className="motion-safe:animate-spin" />
                                    Adding…
                                </>
                            ) : (
                                'Add source'
                            )}
                        </Button>
                    </DialogFooter>
                </form>
            </DialogContent>
        </Dialog>
    );
}
