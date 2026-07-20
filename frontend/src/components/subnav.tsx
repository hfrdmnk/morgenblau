import { cn } from '@/lib/utils';

export type SubnavItem = { id: string; label: string };

export type SubnavConfig = {
    items: SubnavItem[];
    activeId: string;
    onSelect: (id: string) => void;
    ariaLabel: string;
};

export function Subnav({ items, activeId, onSelect, ariaLabel }: SubnavConfig) {
    return (
        <div
            role="tablist"
            aria-label={ariaLabel}
            className="flex items-center justify-center gap-5"
        >
            {items.map((item) => {
                const isActive = item.id === activeId;
                return (
                    <button
                        key={item.id}
                        type="button"
                        role="tab"
                        aria-selected={isActive}
                        onClick={() => onSelect(item.id)}
                        className={cn(
                            'cursor-pointer text-sm transition-opacity duration-200 ease-out outline-none focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring focus-visible:outline-solid',
                            isActive
                                ? 'font-medium text-foreground'
                                : 'text-muted-foreground opacity-60 hover:opacity-100',
                        )}
                    >
                        {item.label}
                    </button>
                );
            })}
        </div>
    );
}
