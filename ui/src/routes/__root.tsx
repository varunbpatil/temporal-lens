import { createRootRoute } from "@tanstack/react-router";
import { RootLayout } from "./-root-layout";

const Route = createRootRoute({
  head: () => ({
    meta: [{ title: "Temporal Lens" }],
  }),
  component: RootLayout,
});

export { Route };
