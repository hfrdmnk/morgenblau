import { createContext, useContext } from 'react';

export type SurfaceLevel = 0 | 1 | 2;

export const LevelContext = createContext<SurfaceLevel>(1);

export function useSurfaceLevel(): SurfaceLevel {
    return useContext(LevelContext);
}
