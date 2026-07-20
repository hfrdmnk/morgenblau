import { DeleteIcon, QuestionCircleIcon, SpinnerIcon } from '@proicons/react';
import { useCallback, useEffect, useRef, useState } from 'react';

import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

// Two-stage confirm: first click arms, second confirms within a 3s auto-cancel window (click-outside/Escape also cancel).
// A failed delete leaves the button armed so the red state invites a retry; success unmounts the row.
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

    // Runs the 3s auto-cancel and dismissers only while armed and not mid-delete: entering `deleting` tears
    // them down so the in-flight request can't be cancelled from under itself; a failure re-arms via this effect.
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
                <DeleteIcon
                    className={cn(
                        'absolute size-3.5 transition-all duration-200 ease-in-out',
                        armed || deleting
                            ? 'scale-50 opacity-0 blur-[3px]'
                            : 'scale-100 opacity-100 blur-0',
                    )}
                />
                <QuestionCircleIcon
                    className={cn(
                        'absolute size-3.5 transition-all duration-200 ease-in-out',
                        armed && !deleting
                            ? 'scale-100 opacity-100 blur-0'
                            : 'scale-50 opacity-0 blur-[3px]',
                    )}
                />
                <SpinnerIcon
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
