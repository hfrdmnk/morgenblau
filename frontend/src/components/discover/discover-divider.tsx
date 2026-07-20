import type { DividerState } from '@/lib/discover-cut';
import { cn } from '@/lib/utils';

// Discover cut: the in-flow rule slides to full-bleed as a card divides and retracts on merge (index.css keyframe).
export function DiscoverDivider({ state }: { state: DividerState }) {
    return (
        <div
            aria-hidden
            className={cn(
                'border-t border-border transition-[margin] duration-[var(--cut-divider-ms)] ease-[cubic-bezier(0.645,0.045,0.355,1)] motion-reduce:transition-none',
                state === 'full-bleed' ? 'mx-0' : 'mx-6',
                state === 'retracting' && 'animate-discover-divider-retract',
            )}
        />
    );
}
