import type { ReactNode } from 'react';

import { AlertIcon, InboxIcon, LoadingIcon } from '@/components/icons';
import { Frame } from '@/design/frame';
import { Text } from '@/design/text';

type StatusStateProps = {
    title: string;
    description?: string;
    action?: ReactNode;
};

type StateFrameProps = StatusStateProps & {
    icon: ReactNode;
    tone?: 'default' | 'error';
    role?: 'alert' | 'status';
};

function StateFrame({ action, description, icon, role, title, tone = 'default' }: StateFrameProps) {
    return (
        <Frame align="center" gap="m" justify="center" minHeight="state" paddingX="2xl" paddingY="4xl" role={role}>
            <Text as="span" tone={tone === 'error' ? 'destructive' : 'muted'}>
                {icon}
            </Text>
            <Frame gap="2xs" maxWidth="sm">
                <Text align="center" weight="medium">
                    {title}
                </Text>
                {description ? (
                    <Text align="center" size="sm" tone="muted" wrap="pretty">
                        {description}
                    </Text>
                ) : null}
            </Frame>
            {action ? <Frame paddingTop="2xs">{action}</Frame> : null}
        </Frame>
    );
}

function LoadingState({ title, ...props }: Omit<StatusStateProps, 'action'>) {
    return <StateFrame title={title} icon={<LoadingIcon size="2xl" spin />} role="status" {...props} />;
}

function EmptyState(props: StatusStateProps) {
    return <StateFrame icon={<InboxIcon size="2xl" />} {...props} />;
}

function ErrorState(props: StatusStateProps) {
    return <StateFrame icon={<AlertIcon size="2xl" />} role="alert" tone="error" {...props} />;
}

export { EmptyState, ErrorState, LoadingState };
