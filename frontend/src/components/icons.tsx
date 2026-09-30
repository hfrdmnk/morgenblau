import type { ComponentProps } from 'react';

type IconProps = ComponentProps<'svg'>;

const iconProps = {
    'aria-hidden': true,
    fill: 'none',
    focusable: false,
    viewBox: '0 0 24 24',
} as const;

function ArrowRightIcon(props: IconProps) {
    return (
        <svg {...iconProps} {...props}>
            <path
                d="M3 12H21M21 12L14 5M21 12L14 19"
                stroke="currentColor"
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth="1.5"
            />
        </svg>
    );
}

function ExternalLinkIcon(props: IconProps) {
    return (
        <svg {...iconProps} {...props}>
            <path
                d="M17 2H22V7"
                stroke="currentColor"
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth="1.5"
            />
            <path
                d="M21 13V19C21 20.1046 20.1046 21 19 21H5C3.89543 21 3 20.1046 3 19V5C3 3.89543 3.89543 3 5 3H11"
                stroke="currentColor"
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth="1.5"
            />
            <path
                d="M13 11L21.5 2.5"
                stroke="currentColor"
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth="1.5"
            />
        </svg>
    );
}

function LoadingIcon(props: IconProps) {
    return (
        <svg {...iconProps} {...props}>
            <path
                d="M16 16L19 19M18 12H22M8 8L5 5M16 8L19 5M8 16L5 19M2 12H6M12 2V6M12 18V22"
                stroke="currentColor"
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth="1.5"
            />
        </svg>
    );
}

function InboxIcon(props: IconProps) {
    return (
        <svg {...iconProps} {...props}>
            <path
                d="M3.04819 12H8.44444L10.2222 14H13.7778L15.5556 12H20.9361M6.70951 5.4902L3.27942 11.2785C3.09651 11.5871 3 11.9393 3 12.2981V17C3 18.1046 3.89543 19 5 19H19C20.1046 19 21 18.1046 21 17V12.2981C21 11.9393 20.9035 11.5871 20.7206 11.2785L17.2905 5.4902C17.1104 5.18633 16.7834 5 16.4302 5H7.5698C7.21659 5 6.88958 5.18633 6.70951 5.4902Z"
                stroke="currentColor"
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth="1.5"
            />
        </svg>
    );
}

function AlertIcon(props: IconProps) {
    return (
        <svg {...iconProps} {...props}>
            <path
                d="M2.20164 18.4695L10.1643 4.00506C10.9021 2.66498 13.0979 2.66498 13.8357 4.00506L21.7984 18.4695C22.4443 19.6428 21.4598 21 19.9627 21H4.0373C2.54022 21 1.55571 19.6428 2.20164 18.4695Z"
                stroke="currentColor"
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth="1.5"
            />
            <path
                d="M12 9V13"
                stroke="currentColor"
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth="1.5"
            />
            <path
                d="M12 17.0195V17"
                stroke="currentColor"
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth="1.5"
            />
        </svg>
    );
}

function CheckIcon(props: IconProps) {
    return (
        <svg {...iconProps} {...props}>
            <path
                d="M3 12L9 18L21 6"
                stroke="currentColor"
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth="1.5"
            />
        </svg>
    );
}

function ChevronDownIcon(props: IconProps) {
    return (
        <svg {...iconProps} {...props}>
            <path
                d="M5 9L12 16L19 9"
                stroke="currentColor"
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth="1.5"
            />
        </svg>
    );
}

function DigestIcon(props: IconProps) {
    return (
        <svg
            aria-hidden="true"
            focusable="false"
            viewBox="-8.68 -10 52 52"
            {...props}
        >
            <path
                d="M29.914 38.75C26.183 40.82 21.889 42 17.32 42 12.75 42 8.459 40.82 4.726 38.75H29.914ZM39.841 29C38.425 31.447 36.622 33.641 34.516 35.5H0.124C-1.982 33.641-3.784 31.447-5.199 29H39.841ZM17.32-10C31.68-10 43.32 1.64 43.32 16 43.32 18.244 43.036 20.422 42.501 22.5H-7.861C-8.396 20.422-8.68 18.244-8.68 16-8.68 1.64 2.961-10 17.32-10Z"
                fill="currentColor"
            />
        </svg>
    );
}

function SourcesIcon(props: IconProps) {
    return (
        <svg
            aria-hidden="true"
            focusable="false"
            viewBox="-9.811 76.475 18.504 16.025"
            {...props}
        >
            <path
                d="M4.756 85.727L0.865 92.418H-9.768L-0.56 76.588 4.756 85.727ZM7.212 89.948L5.776 92.418H3.32L5.984 87.838 7.212 89.948ZM8.649 92.418H7.004L7.826 91.004 8.649 92.418Z"
                fill="currentColor"
            />
        </svg>
    );
}

function LibraryIcon(props: IconProps) {
    return (
        <svg aria-hidden="true" focusable="false" viewBox="-4.68 106 48 48" {...props}>
            <path
                d="M22.784 154.25H-4.93V105.75H22.784V154.25ZM36.641 154.25H29.713V105.75H36.641V154.25ZM43.57 154.25H40.106V105.75H43.57V154.25Z"
                fill="currentColor"
            />
        </svg>
    );
}

function PlusIcon(props: IconProps) {
    return (
        <svg {...iconProps} {...props}>
            <path
                d="M12 5V19M5 12H19"
                stroke="currentColor"
                strokeLinecap="round"
                strokeWidth="1.5"
            />
        </svg>
    );
}

function UploadIcon(props: IconProps) {
    return (
        <svg {...iconProps} {...props}>
            <path
                d="M12 21V7M12 7L6 13M12 7L18 13M3 3H21"
                stroke="currentColor"
                strokeWidth="1.5"
                strokeLinecap="round"
                strokeLinejoin="round"
            />
        </svg>
    );
}

function DownloadIcon(props: IconProps) {
    return (
        <svg {...iconProps} {...props}>
            <path
                d="M12 3V17M12 17L6 11M12 17L18 11M3 21H21"
                stroke="currentColor"
                strokeWidth="1.5"
                strokeLinecap="round"
                strokeLinejoin="round"
            />
        </svg>
    );
}

export {
    AlertIcon,
    ArrowRightIcon,
    CheckIcon,
    ChevronDownIcon,
    DigestIcon,
    DownloadIcon,
    ExternalLinkIcon,
    InboxIcon,
    LibraryIcon,
    LoadingIcon,
    PlusIcon,
    SourcesIcon,
    UploadIcon,
};
