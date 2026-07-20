import { useState } from 'react';

import { InputError } from '@/components/input-error';
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

// ShareComposer collects an optional note before sharing; un-sharing skips this and DELETEs directly from the rail button.
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
                        aria-label="Note"
                        maxLength={3000}
                    />
                ) : null}

                {share.error === 'reauth' ? (
                    <p
                        role="status"
                        className="text-sm font-light text-muted-foreground"
                    >
                        Your session is out of date.{' '}
                        {/* Native anchor: reauth exits the authed shell, which app.tsx assumes is a full server round trip. */}
                        <a
                            href={PATHS.login}
                            className="text-primary underline underline-offset-4"
                        >
                            Sign in again
                        </a>{' '}
                        to share to the Atmosphere.
                    </p>
                ) : share.error === 'failed' ? (
                    <InputError message="Couldn't share just now. Try again." />
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
