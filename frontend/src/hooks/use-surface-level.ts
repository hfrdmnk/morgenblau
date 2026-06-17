import { createContext, useContext } from 'react';

export type SurfaceLevel = 0 | 1;

export const LevelContext = createContext<SurfaceLevel>(0);

export function useSurfaceLevel(): SurfaceLevel {
    return useContext(LevelContext);
}
