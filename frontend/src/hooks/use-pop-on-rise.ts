import { useState } from 'react';

// Adjust-during-render: pops exactly once each time `active` flips false to true, so a badge
// can animate on the flip without replaying on later re-renders.
export function usePopOnRise(active: boolean): {
    pop: boolean;
    endPop: () => void;
} {
    const [prevActive, setPrevActive] = useState(active);
    const [pop, setPop] = useState(false);
    if (active !== prevActive) {
        setPrevActive(active);
        if (active) setPop(true);
    }
    return { pop, endPop: () => setPop(false) };
}
