import { Route } from 'wouter';

import { Styleguide } from '@/dev/styleguide';

export function DevRoutes() {
    return (
        <Route path="/dev/styleguide">
            <Styleguide />
        </Route>
    );
}
