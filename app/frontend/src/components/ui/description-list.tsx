import type { ComponentProps } from 'react';

// Label and value pairs, each group a Frame of its dt and dd; Frame has no dl of its own.
function DescriptionList(props: Omit<ComponentProps<'dl'>, 'className' | 'style'>) {
    return <dl data-slot="description-list" className="flex flex-col gap-3" {...props} />;
}

export { DescriptionList };
