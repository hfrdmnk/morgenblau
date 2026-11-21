import { cva, type VariantProps } from 'class-variance-authority';
import type { ComponentProps, ReactNode } from 'react';

// The reader's actions: a column fixed to the right edge from sm up, a bar along the bottom on phones. Each shows the reading progress.
function RailColumn({ label, progress, children }: { label: string; progress: number; children: ReactNode }) {
    return (
        <aside aria-label={label} className="pointer-events-none fixed top-1/2 right-4 z-10 hidden -translate-y-1/2 sm:right-6 sm:block">
            <div className="pointer-events-auto flex flex-col items-center gap-2">
                {children}
                <ProgressRing progress={progress} />
            </div>
        </aside>
    );
}

function RailBar({ label, progress, children }: { label: string; progress: number; children: ReactNode }) {
    return (
        <aside aria-label={label} className="fixed inset-x-0 bottom-0 z-10 border-t border-border bg-bar pb-(--safe-bottom) backdrop-blur sm:hidden">
            <ProgressLine progress={progress} />
            <div className="flex items-center justify-around px-4 py-1">{children}</div>
        </aside>
    );
}

// Relative, so an icon crossfade inside can stack its layers; touch-sized on phones. The class goes after the spread props because a
// tooltip trigger renders these and merges its own props in.
const railActionVariants = cva(
    'relative inline-flex size-11 cursor-pointer items-center justify-center rounded-md transition-[color,transform] duration-(--motion-duration-fast) ease-out motion-safe:active:scale-96 sm:size-10',
    {
        variants: {
            tone: {
                idle: 'text-muted-foreground hover:text-foreground',
                // A toggle that is on, its icon filled.
                on: 'text-primary [&_path]:fill-current',
                success: 'text-success',
            },
        },
        defaultVariants: { tone: 'idle' },
    },
);

type RailTone = VariantProps<typeof railActionVariants>;

function RailButton({ tone, type = 'button', ...props }: Omit<ComponentProps<'button'>, 'className' | 'style'> & RailTone) {
    return <button data-slot="rail-button" type={type} {...props} className={railActionVariants({ tone })} />;
}

function RailLink({ tone, ...props }: Omit<ComponentProps<'a'>, 'className' | 'style'> & RailTone) {
    return <a data-slot="rail-link" {...props} className={railActionVariants({ tone })} />;
}

const RING_RADIUS = 8.25;
const RING_CIRCUMFERENCE = 2 * Math.PI * RING_RADIUS;

function ProgressRing({ progress }: { progress: number }) {
    return (
        <div aria-label="Reading progress" aria-valuemax={100} aria-valuemin={0} aria-valuenow={Math.round(progress * 100)} className="mt-2 size-4.5" role="progressbar">
            <svg aria-hidden="true" className="size-full -rotate-90" viewBox="0 0 18 18">
                <circle className="stroke-border" cx="9" cy="9" fill="none" r={RING_RADIUS} strokeWidth="1.5" />
                <circle
                    className="stroke-primary"
                    cx="9"
                    cy="9"
                    fill="none"
                    r={RING_RADIUS}
                    strokeDasharray={RING_CIRCUMFERENCE}
                    strokeDashoffset={RING_CIRCUMFERENCE * (1 - progress)}
                    strokeLinecap="round"
                    strokeWidth="1.5"
                />
            </svg>
        </div>
    );
}

function ProgressLine({ progress }: { progress: number }) {
    return (
        <div aria-label="Reading progress" aria-valuemax={100} aria-valuemin={0} aria-valuenow={Math.round(progress * 100)} className="h-px w-full overflow-hidden bg-border" role="progressbar">
            <div className="h-full w-full origin-left bg-primary" style={{ transform: `scaleX(${progress})` }} />
        </div>
    );
}

export { RailBar, RailButton, RailColumn, RailLink };
