import type { ComponentProps } from 'react';

function DescriptionList(props: Omit<ComponentProps<'dl'>, 'className' | 'style'>) {
    return <dl data-slot="description-list" className="flex flex-col gap-3" {...props} />;
}

export { DescriptionList };
