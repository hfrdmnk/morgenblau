import type { ComponentProps } from 'react';

type ProgressBarProps = Omit<ComponentProps<'div'>, 'className' | 'style' | 'children' | 'role'> & {
    value: number;
    max: number;
};

function ProgressBar({ value, max, ...props }: ProgressBarProps) {
    return (
        <div
            data-slot="progress-bar"
            className="h-1.5 overflow-hidden rounded-sm bg-track"
            role="progressbar"
            aria-valuenow={value}
            aria-valuemin={0}
            aria-valuemax={max}
            {...props}
        >
            <div
                className="h-full w-full rounded-sm bg-neutral transition-transform duration-(--motion-duration-overlay) ease-in-out motion-reduce:transition-none"
                style={{ transform: `translateX(${(value / max - 1) * 100}%)` }}
            />
        </div>
    );
}

export { ProgressBar };
