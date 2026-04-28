import type { SVGAttributes } from 'react';

export default function AppLogoIcon(props: SVGAttributes<SVGElement>) {
    return (
        <svg
            {...props}
            viewBox="0 0 64 57"
            fill="currentColor"
            xmlns="http://www.w3.org/2000/svg"
            aria-hidden="true"
        >
            <g>
                <path d="M32.0006 0C21.0913 0 12.2476 8.84376 12.2476 19.7531H51.7537C51.7537 8.84376 42.91 0 32.0006 0Z" />
            </g>
            <g>
                <path d="M47.4132 52.5445C42.8413 55.0621 37.5881 56.4951 32 56.4951C26.4119 56.4951 21.1587 55.0621 16.5868 52.5445H47.4132Z" />
                <path d="M59.8318 40.2976C58.2435 43.089 56.2473 45.618 53.9244 47.8038H10.0756C7.7527 45.618 5.75655 43.089 4.16821 40.2976H59.8318Z" />
                <path d="M64 24.4951C64 28.3833 63.3063 32.1095 62.0363 35.5568H1.96373C0.693729 32.1095 -3.39919e-07 28.3833 0 24.4951H64Z" />
            </g>
        </svg>
    );
}
