import { Route } from 'wouter';

import { Styleguide } from '@/dev/styleguide';

export function DevRoutes() {
    return (
        <Route path="/__dev/styleguide">
            <Styleguide />
        </Route>
    );
}
