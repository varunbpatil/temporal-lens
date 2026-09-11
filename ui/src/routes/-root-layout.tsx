import { HeadContent, Link, Outlet, useRouterState } from "@tanstack/react-router";
import { GlobeIcon, MonitorIcon, MoonIcon, SearchIcon, SunIcon, TelescopeIcon } from "lucide-react";
import { useTheme } from "next-themes";
import { useState } from "react";
import { timeZoneRegions, useTimeZone } from "@/lib/timezone";

import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarRail,
} from "@/components/ui/sidebar";

type ThemePreference = "light" | "dark" | "system";

/** Navigation entries keep the sidebar declarative as domain routes are added. */
const navigationLinks = [{ icon: SearchIcon, label: "Workflow search", to: "/workflows" }] as const;

function TimeZonePicker({
  value,
  onChange,
}: {
  value: string;
  onChange: (timeZone: string) => void;
}) {
  const [open, setOpen] = useState(false);

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <SidebarMenuButton tooltip={`Time zone: ${value}`}>
            <GlobeIcon aria-hidden="true" />
            <span className="truncate">{value}</span>
          </SidebarMenuButton>
        }
      />
      <PopoverContent side="right" align="end" className="w-max max-w-96 p-0">
        <Command>
          <CommandInput placeholder="Filter time zones…" />
          <CommandList>
            <CommandEmpty>No matching time zones.</CommandEmpty>
            {timeZoneRegions.map(({ region, zones }) => (
              <CommandGroup key={region} heading={region}>
                {zones.map((candidate) => (
                  <CommandItem
                    key={candidate.name}
                    value={candidate.name}
                    data-checked={candidate.name === value}
                    onSelect={() => {
                      onChange(candidate.name);
                      setOpen(false);
                    }}
                  >
                    <span className="min-w-0 flex-1 truncate">{candidate.name}</span>
                  </CommandItem>
                ))}
              </CommandGroup>
            ))}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}

/** RootLayout provides the shared sidebar and route outlet. */
export function RootLayout() {
  const { setTheme, theme } = useTheme();
  const { timeZone, setTimeZone } = useTimeZone();
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  });
  const defaultSidebarOpen =
    document.cookie
      .split("; ")
      .find((cookie) => cookie.startsWith("sidebar_state="))
      ?.split("=")[1] !== "false";
  const themePreference: ThemePreference = theme === "light" || theme === "dark" ? theme : "system";
  const themeLabel = themePreference[0].toUpperCase() + themePreference.slice(1);
  const themeIcon =
    themePreference === "light" ? (
      <SunIcon aria-hidden="true" />
    ) : themePreference === "dark" ? (
      <MoonIcon aria-hidden="true" />
    ) : (
      <MonitorIcon aria-hidden="true" />
    );

  return (
    <>
      <HeadContent />
      <SidebarProvider defaultOpen={defaultSidebarOpen}>
        <Sidebar collapsible="icon">
          <SidebarHeader className="h-16 justify-center">
            <SidebarMenu>
              <SidebarMenuItem>
                <SidebarMenuButton render={<Link to="/workflows" />} size="lg">
                  <div className="flex aspect-square size-8 items-center justify-center rounded-lg bg-sidebar-primary text-sidebar-primary-foreground">
                    <TelescopeIcon />
                  </div>
                  <div className="grid flex-1 text-left text-sm leading-tight">
                    <span className="truncate font-medium">Temporal Lens</span>
                  </div>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarHeader>
          <SidebarContent>
            <SidebarGroup>
              <SidebarMenu>
                {navigationLinks.map((link) => {
                  const Icon = link.icon;
                  const isActive = pathname === link.to || pathname.startsWith(`${link.to}/`);

                  return (
                    <SidebarMenuItem key={link.to}>
                      <SidebarMenuButton
                        render={<Link to={link.to} />}
                        isActive={isActive}
                        tooltip={link.label}
                      >
                        <Icon />
                        <span>{link.label}</span>
                      </SidebarMenuButton>
                    </SidebarMenuItem>
                  );
                })}
              </SidebarMenu>
            </SidebarGroup>
          </SidebarContent>
          <SidebarFooter>
            <SidebarMenu>
              <SidebarMenuItem>
                <TimeZonePicker value={timeZone} onChange={setTimeZone} />
              </SidebarMenuItem>
              <SidebarMenuItem>
                <DropdownMenu>
                  <DropdownMenuTrigger
                    render={
                      <SidebarMenuButton tooltip={`Theme: ${themePreference}`}>
                        {themeIcon}
                        <span>{themeLabel}</span>
                      </SidebarMenuButton>
                    }
                  />
                  <DropdownMenuContent side="right" align="end">
                    <DropdownMenuRadioGroup value={themePreference} onValueChange={setTheme}>
                      <DropdownMenuLabel>Appearance</DropdownMenuLabel>
                      <DropdownMenuRadioItem value="light">
                        <SunIcon aria-hidden="true" /> Light
                      </DropdownMenuRadioItem>
                      <DropdownMenuRadioItem value="dark">
                        <MoonIcon aria-hidden="true" /> Dark
                      </DropdownMenuRadioItem>
                      <DropdownMenuRadioItem value="system">
                        <MonitorIcon aria-hidden="true" /> System
                      </DropdownMenuRadioItem>
                    </DropdownMenuRadioGroup>
                  </DropdownMenuContent>
                </DropdownMenu>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarFooter>
          <SidebarRail />
        </Sidebar>
        <SidebarInset>
          <Outlet />
        </SidebarInset>
      </SidebarProvider>
    </>
  );
}
