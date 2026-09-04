import { Outlet, createRootRoute } from "@tanstack/react-router";

const Route = createRootRoute({
  component: () => <Outlet />,
});

export { Route };
