import { PauseIcon, PlayIcon, SpinnerIcon } from '@proicons/react';
import { useState } from 'react';

import { Button } from '@/components/ui/button';
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogHeader,
    DialogTitle,
} from '@/components/ui/dialog';

export function StopNewsletterButton({
    onConfirm,
}: {
    onConfirm: () => Promise<boolean>;
}) {
    const [open, setOpen] = useState(false);
    const [busy, setBusy] = useState(false);

    const confirm = async () => {
        setBusy(true);
        const ok = await onConfirm();
        if (ok) setOpen(false);
        setBusy(false);
    };

    return (
        <Dialog open={open} onOpenChange={setOpen}>
            <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={() => setOpen(true)}
                className="text-muted-foreground"
            >
                <PauseIcon className="size-4" />
                Stop
            </Button>
            <DialogContent>
                <DialogHeader>
                    <DialogTitle>Stop this newsletter?</DialogTitle>
                    <DialogDescription>
                        Unsaved issues will be deleted. Saved issues stay in
                        your library, and future deliveries are discarded until
                        you re-enable the newsletter.
                    </DialogDescription>
                </DialogHeader>
                <DialogFooter>
                    <Button
                        type="button"
                        variant="secondary"
                        disabled={busy}
                        onClick={() => setOpen(false)}
                    >
                        Cancel
                    </Button>
                    <Button type="button" disabled={busy} onClick={confirm}>
                        {busy ? (
                            <>
                                <SpinnerIcon className="size-4 motion-safe:animate-spin" />
                                Stopping…
                            </>
                        ) : (
                            'Stop newsletter'
                        )}
                    </Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

export function EnableNewsletterButton({
    onEnable,
}: {
    onEnable: () => Promise<void>;
}) {
    const [busy, setBusy] = useState(false);

    return (
        <Button
            type="button"
            variant="secondary"
            size="sm"
            disabled={busy}
            onClick={async () => {
                setBusy(true);
                try {
                    await onEnable();
                } finally {
                    setBusy(false);
                }
            }}
        >
            {busy ? (
                <SpinnerIcon className="size-4 motion-safe:animate-spin" />
            ) : (
                <PlayIcon className="size-4" />
            )}
            Re-enable
        </Button>
    );
}
