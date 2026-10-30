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

function EnterIcon(props: IconProps) {
    return (
        <svg {...iconProps} {...props}>
            <path
                d="M8 11L4 15M4 15L8 19M4 15H17C18.6569 15 20 13.6569 20 12V5"
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

function InfoIcon(props: IconProps) {
    return (
        <svg {...iconProps} {...props}>
            <path
                d="M22 12C22 17.5228 17.5228 22 12 22C6.47715 22 2 17.5228 2 12C2 6.47715 6.47715 2 12 2C17.5228 2 22 6.47715 22 12Z"
                stroke="currentColor"
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth="1.5"
            />
            <path
                d="M12 10V18"
                stroke="currentColor"
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth="1.5"
            />
            <path
                d="M12 6.01953V6"
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
        <svg
            aria-hidden="true"
            focusable="false"
            viewBox="-4.68 106 48 48"
            {...props}
        >
            <path
                d="M22.784 154.25H-4.93V105.75H22.784V154.25ZM36.641 154.25H29.713V105.75H36.641V154.25ZM43.57 154.25H40.106V105.75H43.57V154.25Z"
                fill="currentColor"
            />
        </svg>
    );
}

function SettingsIcon(props: IconProps) {
    return (
        <svg {...iconProps} {...props}>
            <path
                d="M4.08923 6.24364L11.4377 2.26004C12.1113 1.89488 12.935 1.91558 13.5885 2.31409L20.0047 6.22711C20.6239 6.60472 21 7.26692 21 7.97938V16.0206C21 16.7331 20.6239 17.3953 20.0047 17.7729L13.5885 21.6859C12.935 22.0844 12.1113 22.1051 11.4377 21.74L4.08923 17.7564C3.41695 17.3919 3 16.7005 3 15.95V8.04996C3 7.29953 3.41695 6.60808 4.08923 6.24364Z"
                stroke="currentColor"
                strokeWidth="1.5"
                strokeLinecap="round"
                strokeLinejoin="round"
            />
            <path
                d="M15 12.0002C15 13.657 13.6569 15.0002 12 15.0002C10.3431 15.0002 9 13.657 9 12.0002C9 10.3433 10.3431 9.00016 12 9.00016C13.6569 9.00016 15 10.3433 15 12.0002Z"
                stroke="currentColor"
                strokeWidth="1.5"
                strokeLinecap="round"
                strokeLinejoin="round"
            />
        </svg>
    );
}

function ShieldIcon(props: IconProps) {
    return (
        <svg {...iconProps} {...props}>
            <path
                d="M12 2L4 5V11C4 16 7.4 20.4 12 22C16.6 20.4 20 16 20 11V5L12 2Z"
                stroke="currentColor"
                strokeWidth="1.5"
                strokeLinecap="round"
                strokeLinejoin="round"
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

function CloseIcon(props: IconProps) {
    return (
        <svg {...iconProps} {...props}>
            <path
                d="M5 5L19 19M5 19L19 5"
                stroke="currentColor"
                strokeWidth="1.5"
                strokeLinecap="round"
                strokeLinejoin="round"
            />
        </svg>
    );
}

function CopyIcon(props: IconProps) {
    return (
        <svg {...iconProps} {...props}>
            <path
                d="M5 15H4C2.89543 15 2 14.1046 2 13V4C2 2.89543 2.89543 2 4 2H13C14.1046 2 15 2.89543 15 4V5M11 22H20C21.1046 22 22 21.1046 22 20V11C22 9.89543 21.1046 9 20 9H11C9.89543 9 9 9.89543 9 11V20C9 21.1046 9.89543 22 11 22Z"
                stroke="currentColor"
                strokeWidth="1.5"
                strokeLinecap="round"
                strokeLinejoin="round"
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

function MoreIcon(props: IconProps) {
    return (
        <svg {...iconProps} {...props}>
            <path
                d="M7 12C7 12.5523 6.55228 13 6 13C5.44772 13 5 12.5523 5 12C5 11.4477 5.44772 11 6 11C6.55228 11 7 11.4477 7 12Z"
                stroke="currentColor"
                strokeWidth="1.5"
                strokeLinecap="round"
                strokeLinejoin="round"
            />
            <path
                d="M13 12C13 12.5523 12.5523 13 12 13C11.4477 13 11 12.5523 11 12C11 11.4477 11.4477 11 12 11C12.5523 11 13 11.4477 13 12Z"
                stroke="currentColor"
                strokeWidth="1.5"
                strokeLinecap="round"
                strokeLinejoin="round"
            />
            <path
                d="M19 12C19 12.5523 18.5523 13 18 13C17.4477 13 17 12.5523 17 12C17 11.4477 17.4477 11 18 11C18.5523 11 19 11.4477 19 12Z"
                stroke="currentColor"
                strokeWidth="1.5"
                strokeLinecap="round"
                strokeLinejoin="round"
            />
        </svg>
    );
}

function PauseIcon(props: IconProps) {
    return (
        <svg {...iconProps} {...props}>
            <path
                d="M6 5C6 4.44772 6.44772 4 7 4H9C9.55228 4 10 4.44772 10 5V19C10 19.5523 9.55228 20 9 20H7C6.44772 20 6 19.5523 6 19V5Z"
                stroke="currentColor"
                strokeWidth="1.5"
                strokeLinecap="round"
                strokeLinejoin="round"
            />
            <path
                d="M14 5C14 4.44772 14.4477 4 15 4H17C17.5523 4 18 4.44772 18 5V19C18 19.5523 17.5523 20 17 20H15C14.4477 20 14 19.5523 14 19V5Z"
                stroke="currentColor"
                strokeWidth="1.5"
                strokeLinecap="round"
                strokeLinejoin="round"
            />
        </svg>
    );
}

function PenIcon(props: IconProps) {
    return (
        <svg {...iconProps} {...props}>
            <path
                d="M3.9933 17.0268L17.4029 3.61716C18.2258 2.79428 19.56 2.79428 20.3828 3.61716C21.2057 4.44004 21.2057 5.7742 20.3828 6.59708L6.97322 20.0067L3 21L3.9933 17.0268Z"
                stroke="currentColor"
                strokeWidth="1.5"
                strokeLinecap="round"
                strokeLinejoin="round"
            />
        </svg>
    );
}

function PlayIcon(props: IconProps) {
    return (
        <svg {...iconProps} {...props}>
            <path
                d="M5 19.1684V4.83171C5 4.04055 5.87525 3.56271 6.54076 3.99054L17.6915 11.1589C18.3038 11.5525 18.3038 12.4476 17.6915 12.8412L6.54076 20.0096C5.87525 20.4374 5 19.9595 5 19.1684Z"
                stroke="currentColor"
                strokeWidth="1.5"
                strokeLinecap="round"
                strokeLinejoin="round"
            />
        </svg>
    );
}

function TagIcon(props: IconProps) {
    return (
        <svg {...iconProps} {...props}>
            <path
                d="M7.01611 7.01611L7 7M3 3L3 12.393L11.0512 20.4442C11.7922 21.1853 12.9938 21.1853 13.7349 20.4442L20.4442 13.7349C21.1853 12.9938 21.1853 11.7922 20.4442 11.0512L12.393 3L3 3Z"
                stroke="currentColor"
                strokeWidth="1.5"
                strokeLinecap="round"
                strokeLinejoin="round"
            />
        </svg>
    );
}

function TrashIcon(props: IconProps) {
    return (
        <svg {...iconProps} {...props}>
            <path
                d="M9 3H15M3 6H21M10 16V11M14 16V11M5 6H19L18.1245 19.133C18.0544 20.1836 17.1818 21 16.1289 21L7.8461 21C6.79171 21 5.91842 20.1814 5.85028 19.1292L5 6Z"
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
    CloseIcon,
    CopyIcon,
    DigestIcon,
    DownloadIcon,
    EnterIcon,
    ExternalLinkIcon,
    InboxIcon,
    InfoIcon,
    LibraryIcon,
    LoadingIcon,
    MoreIcon,
    PauseIcon,
    PenIcon,
    PlayIcon,
    PlusIcon,
    SettingsIcon,
    ShieldIcon,
    SourcesIcon,
    TagIcon,
    TrashIcon,
    UploadIcon,
};
