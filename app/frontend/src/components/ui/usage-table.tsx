import type { ComponentProps } from 'react';

type Props<E extends 'dl' | 'div' | 'li'> = Omit<ComponentProps<E>, 'className' | 'style'>;

function UsageStats(props: Props<'dl'>) {
    return <dl data-slot="usage-stats" className="grid grid-cols-2 gap-x-6 gap-y-8 sm:grid-cols-4" {...props} />;
}

// The header's first column spans a row's avatar and name, so the three fixed count columns line up with every row's.
function UsageHeader(props: Props<'div'>) {
    return <div data-slot="usage-header" className="hidden grid-cols-(--grid-usage) gap-4 sm:grid" {...props} />;
}

function UsageRow(props: Props<'li'>) {
    return (
        <li
            data-slot="usage-row"
            className="grid min-w-0 grid-cols-(--grid-usage-row) items-center gap-x-3 gap-y-2 sm:grid-cols-(--grid-usage-row-wide) sm:gap-x-4"
            {...props}
        />
    );
}

export { UsageHeader, UsageRow, UsageStats };
