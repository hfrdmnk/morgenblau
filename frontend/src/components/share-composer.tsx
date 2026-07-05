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
import { Textarea } from '@/components/ui/textarea';
import type { ShareControl } from '@/hooks/use-share-toggle';
import { PATHS } from '@/lib/paths';

// The share composer: an optional note before the item reaches the user's
// network and the Atmosphere. Un-sharing never opens this — it's a direct
// DELETE from the rail button.
export function ShareComposer({ share }: { share: ShareControl }) {
    const [comment, setComment] = useState('');
    const [wasOpen, setWasOpen] = useState(share.composerOpen);

    // Reset the note each time the composer opens (adjust-state-during-render).
    if (share.composerOpen !== wasOpen) {
        setWasOpen(share.composerOpen);
        if (share.composerOpen) setComment('');
    }

    return (
        <Dialog
            open={share.composerOpen}
            onOpenChange={(open) => {
                if (!open) share.closeComposer();
            }}
        >
            <DialogContent>
                <DialogHeader>
                    <DialogTitle>Share this</DialogTitle>
                    <DialogDescription>
                        {share.canComment
                            ? 'Add a note, or share as-is. It reaches your network and the Atmosphere.'
                            : 'Share this with your network and the Atmosphere.'}
                    </DialogDescription>
                </DialogHeader>

                {share.canComment ? (
                    <Textarea
                        value={comment}
                        onChange={(e) => setComment(e.target.value)}
                        placeholder="Say something (optional)"
                        maxLength={3000}
                    />
                ) : null}

                {share.error === 'reauth' ? (
                    <p className="text-sm font-light text-muted-foreground">
                        Your session is out of date.{' '}
                        <a
                            href={PATHS.login}
                            className="text-primary underline underline-offset-4"
                        >
                            Sign in again
                        </a>{' '}
                        to share to the Atmosphere.
                    </p>
                ) : share.error === 'failed' ? (
                    <p className="text-sm font-light text-muted-foreground">
                        Couldn't share just now. Try again.
                    </p>
                ) : null}

                <DialogFooter>
                    <Button
                        variant="secondary"
                        onClick={() => share.closeComposer()}
                        disabled={share.busy}
                    >
                        Cancel
                    </Button>
                    <Button
                        onClick={() => share.submit(comment)}
                        disabled={share.busy}
                    >
                        {share.busy ? 'Sharing…' : 'Share'}
                    </Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}
