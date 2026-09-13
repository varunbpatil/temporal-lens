import { Link } from "@tanstack/react-router";
import { ArrowLeftIcon } from "lucide-react";

import { Button } from "@/components/ui/button";

/** NotFoundPage is rendered inside the application shell for unknown UI routes. */
export function NotFoundPage() {
  return (
    <main className="flex min-h-full flex-1 items-center justify-center p-6 sm:p-10">
      <section className="text-center">
        <h1 className="text-3xl font-semibold tracking-tight">Not Found</h1>
        <Button className="mt-6" render={<Link to="/" />}>
          <ArrowLeftIcon aria-hidden="true" />
          Go back home
        </Button>
      </section>
    </main>
  );
}
