import { toast } from 'sonner';

import { LinkToast } from '@/components/ui/sonner';
import { sourceHref } from '@/lib/paths';

export function announceSourceAdded(
    rkey: string,
    navigate: (to: string) => void,
    description = 'Morgenblau is fetching its latest posts.',
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
