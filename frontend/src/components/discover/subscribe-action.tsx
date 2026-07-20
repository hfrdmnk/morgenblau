import { AddIcon, CheckmarkIcon } from '@proicons/react';

import { Button } from '@/components/ui/button';
import { usePopOnRise } from '@/hooks/use-pop-on-rise';
import { cn } from '@/lib/utils';

// SPEC <discovery>: subscribing flips the row to an inert state in place; the badge pops once on the flip.
export function SubscribeAction({
    subscribed,
    onSubscribe,
    size = 'sm',
}: {
    subscribed: boolean;
    onSubscribe: () => void;
    size?: 'sm' | 'xs';
}) {
    const { pop, endPop } = usePopOnRise(subscribed);
    if (!subscribed) {
        return (
            <Button variant="ghost" size={size} iconTint="primary" onClick={onSubscribe}>
                <AddIcon />
                Subscribe
            </Button>
        );
    }
    return <InertBadge label="Subscribed" size={size} pop={pop} onPopEnd={endPop} />;
}

export function InertBadge({
    label,
    size = 'sm',
    pop,
    onPopEnd,
}: {
    label: string;
    size?: 'sm' | 'xs';
    pop: boolean;
    onPopEnd: () => void;
}) {
    return (
        <Button
            type="button"
            variant="ghost"
            size={size}
            iconTint="success"
            disabled
            className={cn(
                'disabled:cursor-default disabled:opacity-100 disabled:saturate-100',
                pop && 'motion-safe:animate-discover-subscribe-pop',
            )}
            onAnimationEnd={onPopEnd}
        >
            <CheckmarkIcon />
            {label}
        </Button>
    );
}
