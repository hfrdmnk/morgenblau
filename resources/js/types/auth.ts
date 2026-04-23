export type User = {
    did: string;
    iss: string | null;
    created_at: string;
    updated_at: string;
};

export type Auth = {
    user: User | null;
    handle: string | null;
};
