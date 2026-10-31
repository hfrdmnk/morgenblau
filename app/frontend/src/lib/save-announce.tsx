import { toast } from 'sonner';

import { SaveToast } from '@/components/save-toast';
import type { EntrySave } from '@/lib/entry-save';

const SAVE_TOAST = 'entry-save';
const REMOVED_TOAST = 'entry-save-removed';

// Focus inside the toast holds it open; leaving it gives a short grace instead of the full duration.
export function announceSave(save: EntrySave) {
    toast.dismiss(REMOVED_TOAST);
    const show = (duration: number) => toast.custom(() => <SaveToast onHold={(held) => show(held ? Infinity : 3000)} save={save} />, { id: SAVE_TOAST, duration });
    show(5000);
}

export function announceRemove(save: EntrySave, tags: string[]) {
    save.remove();
    toast.dismiss(SAVE_TOAST);
    toast('Removed from Library', {
        id: REMOVED_TOAST,
        action: { label: 'Undo', onClick: () => (tags.length ? save.setTags(tags) : save.save()) },
        classNames: { actionButton: '-my-1 ml-auto shrink-0 cursor-pointer rounded-sm px-2 py-1 text-sm font-medium hover:bg-secondary' },
    });
}

export function dismissSaveToasts() {
    toast.dismiss(SAVE_TOAST);
    toast.dismiss(REMOVED_TOAST);
}
