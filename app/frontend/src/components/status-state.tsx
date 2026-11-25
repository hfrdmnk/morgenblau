import type { ReactNode } from 'react';

import { LoadingIcon } from '@/components/icons';
import { StateSymbol, type StateSymbolShape } from '@/components/ui/state-symbol';
import { Frame } from '@/design/frame';
import { Text } from '@/design/text';

type StatusStateProps = {
    title: string;
    description?: string;
    action?: ReactNode;
};

type StateFrameProps = StatusStateProps & {
    icon: ReactNode;
    gap: 'm' | 'xl';
    role?: 'alert' | 'status';
};

function StateFrame({ action, description, gap, icon, role, title }: StateFrameProps) {
    return (
        <Frame align="center" gap={gap} justify="center" minHeight="state" paddingX="2xl" paddingY="4xl" role={role}>
            {icon}
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
    return (
        <StateFrame
            title={title}
            icon={<LoadingIcon size="2xl" spin tone="muted" />}
            gap="m"
            role="status"
            {...props}
        />
    );
}

function EmptyState({ symbol, ...props }: StatusStateProps & { symbol: StateSymbolShape }) {
    return <StateFrame icon={<StateSymbol shape={symbol} />} gap="xl" {...props} />;
}

function ErrorState(props: StatusStateProps) {
    return <StateFrame icon={<StateSymbol shape="loop" />} gap="xl" role="alert" {...props} />;
}

export { EmptyState, ErrorState, LoadingState };
