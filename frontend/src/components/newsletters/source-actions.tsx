import { PencilIcon } from '@proicons/react';

import {
    EnableNewsletterButton,
    StopNewsletterButton,
} from '@/components/newsletters/lifecycle-button';
import { Button } from '@/components/ui/button';
import { Separator } from '@/components/ui/separator';
import type { NewsletterSource } from '@/lib/newsletters';

export function NewsletterSourceActions({
    status,
    onEdit,
    onStop,
    onEnable,
}: {
    status: NewsletterSource['status'];
    onEdit: () => void;
    onStop: () => Promise<boolean>;
    onEnable: () => Promise<void>;
}) {
    if (status === 'stopped') {
        return <EnableNewsletterButton onEnable={onEnable} />;
    }

    return (
        <>
            <Button
                variant="ghost"
                size="icon-sm"
                aria-label="Edit newsletter"
                className="text-muted-foreground"
                onClick={onEdit}
            >
                <PencilIcon className="size-3.5" />
            </Button>
            <Separator orientation="vertical" className="h-5" />
            <StopNewsletterButton onConfirm={onStop} />
        </>
    );
}
