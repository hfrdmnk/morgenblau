import { toast } from 'sonner';

import { LinkToast } from '@/components/ui/sonner';
import { sourceHref } from '@/lib/paths';

// The whole toast is the link, so it reads as one target instead of a message with a button beside it.
export function announceSourceAdded(
    rkey: string,
    navigate: (to: string) => void,
    description = 'Their latest entries are being collected.',
) {
    toast.custom((id) => (
        <LinkToast
            description={description}
            onClick={() => {
                toast.dismiss(id);
                navigate(sourceHref(rkey));
            }}
            title="Source added"
        />
    ));
}
