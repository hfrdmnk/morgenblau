import { useKeyboard, type KeyMap } from '@/hooks/use-keyboard';
import type { ListNavigation } from '@/hooks/use-list-navigation';

// Base arrow/Enter/Escape wiring shared by every keyboard-navigable list; callers layer extra keys on top.
export function useListNavKeyboard(nav: ListNavigation, extraKeys?: KeyMap) {
    useKeyboard({
        ArrowDown: () => nav.move(1),
        ArrowUp: () => nav.move(-1),
        Enter: () => nav.open(),
        Escape: () => nav.clear(),
        ...extraKeys,
    });
}
