import type { ReactNode } from 'react';

import { AlertIcon, InboxIcon, LoadingIcon } from '@/components/icons';
import { cn } from '@/lib/utils';

type StatusStateProps = {
    title: string;
    description?: string;
    action?: ReactNode;
    className?: string;
};

type StateFrameProps = StatusStateProps & {
    icon: ReactNode;
    tone?: 'default' | 'error';
    role?: 'alert' | 'status';
};

function StateFrame({
    action,
    className,
    description,
    icon,
    role,
    title,
    tone = 'default',
}: StateFrameProps) {
    return (
        <div
            className={cn(
                'flex min-h-52 flex-col items-center justify-center gap-3 px-6 py-10 text-center',
                className,
            )}
            role={role}
        >
            <span
                className={cn(
                    'text-muted-foreground [&>svg]:size-6',
                    tone === 'error' && 'text-destructive',
                )}
            >
                {icon}
            </span>
            <div className="max-w-sm space-y-1">
                <p className="font-medium">{title}</p>
                {description ? (
                    <p className="text-sm text-muted-foreground">{description}</p>
                ) : null}
            </div>
            {action ? <div className="pt-1">{action}</div> : null}
        </div>
    );
}

function LoadingState({ title, ...props }: Omit<StatusStateProps, 'action'>) {
    return (
        <StateFrame
            title={title}
            icon={<LoadingIcon className="motion-safe:animate-spin" />}
            role="status"
            {...props}
        />
    );
}

function EmptyState(props: StatusStateProps) {
    return <StateFrame icon={<InboxIcon />} {...props} />;
}

function ErrorState(props: StatusStateProps) {
    return (
        <StateFrame
            icon={<AlertIcon />}
            role="alert"
            tone="error"
            {...props}
        />
    );
}

export { EmptyState, ErrorState, LoadingState };
