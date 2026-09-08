# Temporal Lens UI

The UI is a React single-page application for Temporal Lens. It is built with
Vite, TypeScript, TanStack Router, TanStack Query, Buf Connect, Tailwind CSS v4,
and shadcn/ui.

When the Go service is built with UI assets, its HTTP server serves the
production build at `/` and exposes Connect API endpoints below `/api`. During
development, Vite proxies `/api` to the Go server at `http://localhost:8080`.

## Commands

From the repository root:

```sh
make ui/dev    # Start Vite with hot reload
make ui/build  # Type-check and create ui/dist
make ui/lint   # Run Oxlint
make ui/fmt    # Format UI files with Oxfmt
```

Run the workflow service separately when developing against the API. The Vite
proxy assumes its HTTP address is the default `:8080`.

## Structure

```text
ui/
├── public/                         # Files copied unchanged into the build
├── src/
│   ├── assets/                     # Source assets imported by UI code
│   ├── components/
│   │   ├── ui/                     # shadcn/ui primitives; keep CLI-compatible
│   │   └── ...                     # Custom components
│   ├── gen/                        # Generated Buf protobuf and Connect code
│   ├── routes/                     # File-based TanStack Router routes
│   ├── index.css                   # Tailwind v4 and shadcn design tokens
│   ├── main.tsx                    # Application providers and startup
│   └── routeTree.gen.ts            # Generated route tree; do not edit
├── components.json                 # shadcn/ui configuration
├── vite.config.ts                  # Vite, router, Tailwind, aliases, API proxy
└── package.json
```

### Application entry point

`src/main.tsx` creates the application-wide providers, in this order:

1. `TransportProvider` supplies one Buf Connect transport with `/api` as its base URL.
2. `QueryClientProvider` supplies the TanStack Query cache.
3. `RouterProvider` renders the file-based TanStack Router route tree.

Generated Connect Query hooks use the transport from `TransportProvider`. API
requests therefore go to paths such as
`/api/temporal_lens.workflows.v1.WorkflowService/Search` without each feature
needing to configure a client.

### Routes

Routes live in `src/routes`. TanStack Router generates `src/routeTree.gen.ts`
from these files during development and builds. Do not edit the generated file.

### Generated API code

`src/gen` contains TypeScript generated from the repository protobuf files.
Use the generated message types and Connect Query method descriptors instead of
hand-writing API request types. Regenerate them from the repository root:

```sh
make proto/generate
```

## Styling

`src/index.css` is the Tailwind CSS v4 + shadcn theme stylesheet.
Use Tailwind utility classes and shadcn/ui primitives for new UI. Keep
domain-specific composition in feature components rather than changing the
generated primitives in `src/components/ui`.
