export type User = {
    did: string;
    iss: string | null;
    created_at: string;
    updated_at: string;
};

export type Profile = {
    avatar: string | null;
    displayName: string | null;
};

export type Auth = {
    user: User | null;
    handle: string | null;
    profile?: Profile | null;
};
