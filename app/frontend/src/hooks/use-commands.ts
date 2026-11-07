import { useEffect, useState } from 'react';

import { pageCommands, type Command } from '@/lib/commands';

// Offers commands in the command menu for as long as the calling component is mounted, with its latest state.
export function useCommands(commands: Command[]) {
    const [key] = useState(() => Symbol('commands'));
    useEffect(() => pageCommands.set(key, commands));
    useEffect(() => () => pageCommands.delete(key), [key]);
}
