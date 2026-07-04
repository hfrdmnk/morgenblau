import {
    Delete02Icon,
    HelpCircleIcon,
    Loading03Icon,
} from '@hugeicons/core-free-icons';
import { HugeiconsIcon } from '@hugeicons/react';
import { useCallback, useEffect, useRef, useState } from 'react';

import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

// Two-stage icon-morph: first click arms (Delete → red HelpCircle, scale-down
// + blur switch), second click within the window confirms. Auto-cancels after
// 3 s; click-outside or Escape also cancel. While the delete is in flight a
// spinner shows; if it fails the button stays armed so the red state invites a
// retry (on success the parent unmounts the row).
export function DeleteSourceButton({
    onConfirm,
}: {
    onConfirm: () => Promise<boolean>;
}) {
    const [armed, setArmed] = useState(false);
    const [deleting, setDeleting] = useState(false);
    const buttonRef = useRef<HTMLButtonElement | null>(null);
    const timerRef = useRef<number | null>(null);

    const cancel = useCallback(() => {
        setArmed(false);
        if (timerRef.current !== null) {
            window.clearTimeout(timerRef.current);
            timerRef.current = null;
        }
    }, []);

    // While armed (and not mid-delete) run the 3 s auto-cancel and the
    // click-outside / Escape dismissers. Entering `deleting` tears them down so
    // the in-flight request can't be cancelled out from under itself; a failure
    // clears `deleting` and this re-arms the timer.
    useEffect(() => {
        if (!armed || deleting) return;
        const onDocClick = (e: MouseEvent) => {
            if (
                buttonRef.current &&
                !buttonRef.current.contains(e.target as Node)
            ) {
                cancel();
            }
        };
        const onKey = (e: KeyboardEvent) => {
            if (e.key === 'Escape') cancel();
        };
        timerRef.current = window.setTimeout(cancel, 3000);
        document.addEventListener('mousedown', onDocClick);
        document.addEventListener('keydown', onKey);
        return () => {
            document.removeEventListener('mousedown', onDocClick);
            document.removeEventListener('keydown', onKey);
            if (timerRef.current !== null) {
                window.clearTimeout(timerRef.current);
                timerRef.current = null;
            }
        };
    }, [armed, deleting, cancel]);

    const onClick = async () => {
        if (deleting) return;
        if (!armed) {
            setArmed(true);
            return;
        }
        setDeleting(true);
        const ok = await onConfirm();
        if (!ok) setDeleting(false);
    };

    return (
        <Button
            ref={buttonRef}
            variant="ghost"
            size="icon-sm"
            aria-label={armed ? 'Confirm remove' : 'Remove source'}
            aria-pressed={armed}
            disabled={deleting}
            onClick={onClick}
            className={
                armed
                    ? 'text-destructive hover:text-destructive'
                    : 'text-muted-foreground'
            }
        >
            <span className="relative grid size-3.5 place-items-center">
                <HugeiconsIcon
                    icon={Delete02Icon}
                    className={cn(
                        'absolute size-3.5 transition-all duration-200 ease-in-out',
                        armed || deleting
                            ? 'scale-50 opacity-0 blur-[3px]'
                            : 'scale-100 opacity-100 blur-0',
                    )}
                />
                <HugeiconsIcon
                    icon={HelpCircleIcon}
                    className={cn(
                        'absolute size-3.5 transition-all duration-200 ease-in-out',
                        armed && !deleting
                            ? 'scale-100 opacity-100 blur-0'
                            : 'scale-50 opacity-0 blur-[3px]',
                    )}
                />
                <HugeiconsIcon
                    icon={Loading03Icon}
                    className={cn(
                        'absolute size-3.5 transition-all duration-200 ease-in-out motion-safe:animate-spin',
                        deleting
                            ? 'scale-100 opacity-100 blur-0'
                            : 'scale-50 opacity-0 blur-[3px]',
                    )}
                />
            </span>
        </Button>
    );
}
