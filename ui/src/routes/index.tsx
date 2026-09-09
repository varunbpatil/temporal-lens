import { createFileRoute, redirect } from "@tanstack/react-router";

const Route = createFileRoute("/")({
  beforeLoad: () => {
    throw redirect({ to: "/workflows" });
  },
});

export { Route };
