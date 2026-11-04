import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { SkulletProvider } from 'skullet/react';
import './index.css'
import App from './app.tsx';
import { ErrorBoundary } from '@/components/error-boundary';

// Dark mode is the `.dark` class, not the media query, so one colour set follows the tokens in both.
const SKELETON_THEME = {
  light: { base: 'var(--muted)', highlight: 'color-mix(in oklch, var(--background) 55%, transparent)' },
  speed: 'slow',
} as const;

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ErrorBoundary>
      <SkulletProvider mode="light" theme={SKELETON_THEME}>
        <App />
      </SkulletProvider>
    </ErrorBoundary>
  </StrictMode>,
)
