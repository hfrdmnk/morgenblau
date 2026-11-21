import { Accordion as AccordionPrimitive } from '@base-ui/react/accordion';

import { ChevronDownIcon } from '@/components/icons';
import { cn } from 'cn';

function Accordion({ className, tone, ...props }: AccordionPrimitive.Root.Props & { tone?: 'muted' }) {
    return (
        <AccordionPrimitive.Root
            data-slot="accordion"
            className={cn('flex w-full flex-col', tone === 'muted' && 'text-muted-foreground', className)}
            {...props}
        />
    );
}

function AccordionItem({ className, ...props }: AccordionPrimitive.Item.Props) {
    return (
        <AccordionPrimitive.Item data-slot="accordion-item" className={cn('not-last:border-b', className)} {...props} />
    );
}

function AccordionTrigger({ className, children, ...props }: AccordionPrimitive.Trigger.Props) {
    return (
        <AccordionPrimitive.Header className="flex">
            <AccordionPrimitive.Trigger
                data-slot="accordion-trigger"
                className={cn(
                    'group/accordion-trigger relative flex flex-1 cursor-pointer items-center justify-between gap-4 rounded-md py-4 text-left text-sm font-medium hover:underline aria-disabled:pointer-events-none aria-disabled:opacity-50',
                    className,
                )}
                {...props}
            >
                {children}
                <span
                    data-slot="accordion-trigger-icon"
                    className="pointer-events-none flex shrink-0 transition-transform duration-(--motion-duration-ui) ease-in-out group-aria-expanded/accordion-trigger:rotate-180 motion-reduce:transition-none"
                >
                    <ChevronDownIcon size="md" tone="muted" />
                </span>
            </AccordionPrimitive.Trigger>
        </AccordionPrimitive.Header>
    );
}

function AccordionContent({ className, children, ...props }: AccordionPrimitive.Panel.Props) {
    return (
        <AccordionPrimitive.Panel
            data-slot="accordion-content"
            className="overflow-hidden text-sm duration-(--motion-duration-ui) ease-out data-open:animate-accordion-down data-closed:animate-accordion-up motion-reduce:animate-none"
            {...props}
        >
            <div
                className={cn(
                    'pb-4 [&_a]:underline [&_a]:underline-offset-3 [&_a]:hover:text-foreground [&_p:not(:last-child)]:mb-4',
                    className,
                )}
            >
                {children}
            </div>
        </AccordionPrimitive.Panel>
    );
}

export { Accordion, AccordionItem, AccordionTrigger, AccordionContent };
