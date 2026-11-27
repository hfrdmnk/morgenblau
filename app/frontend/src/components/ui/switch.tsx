import { Switch as SwitchPrimitive } from '@base-ui/react/switch';
import type { ComponentProps } from 'react';

function Switch(props: Omit<SwitchPrimitive.Root.Props, 'className' | 'style'>) {
    return (
        <SwitchPrimitive.Root
            data-slot="switch"
            className="relative flex h-6 w-10 shrink-0 cursor-pointer rounded-full bg-accent p-0.5 transition-[background-color] duration-(--motion-duration-ui) ease-(--ease-out) data-checked:bg-primary data-disabled:cursor-not-allowed"
            {...props}
        >
            <SwitchPrimitive.Thumb className="size-5 rounded-full bg-popover shadow-thumb transition-transform duration-(--motion-duration-ui) ease-(--ease-out) data-checked:translate-x-4" />
        </SwitchPrimitive.Root>
    );
}

function SwitchLabel(props: Omit<ComponentProps<'label'>, 'className' | 'style'>) {
    return <label data-slot="switch-label" className="cursor-pointer select-none" {...props} />;
}

export { Switch, SwitchLabel };
