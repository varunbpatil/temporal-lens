import { createRootRoute } from "@tanstack/react-router";
import { RootLayout } from "./-root-layout";
import { NotFoundPage } from "./-not-found";

const Route = createRootRoute({
  head: () => ({
    meta: [{ title: "Temporal Lens" }],
  }),
  component: RootLayout,
  notFoundComponent: NotFoundPage,
});

export { Route };
